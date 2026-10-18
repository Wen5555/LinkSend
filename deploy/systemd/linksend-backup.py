#!/usr/bin/env python3
"""Online SQLite backup and isolated, no-overwrite restore for LinkSend.

Database bytes and membership contents stay on the host. Only paths, hashes and
verification outcomes are printed. Backups include a checksum before retention
can consider them complete; restoring never replaces an existing destination.
"""

from __future__ import annotations

import argparse
from contextlib import closing
import hashlib
import json
import os
from pathlib import Path
import re
import sqlite3
import stat
import time
from datetime import datetime, timezone
import uuid


BACKUP_NAME = re.compile(r"control-\d{8}T\d{6}Z-[a-f0-9]{8}\.db\Z")


def regular_file(path: Path) -> None:
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or path.is_symlink():
        raise ValueError("expected a regular file, without symlinks")


def private_directory(path: Path) -> None:
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    if path.is_symlink() or not path.is_dir():
        raise ValueError("backup/restore directory must not be a symlink")
    if os.name != "nt" and path.stat().st_mode & 0o077:
        raise ValueError("backup/restore directory must have mode 0700")


def digest(path: Path) -> str:
    result = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def sync_directory(path: Path) -> None:
    if os.name == "nt":
        return
    descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def exclusive_text(path: Path, text: str) -> None:
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as stream:
        stream.write(text)
        stream.flush()
        os.fsync(stream.fileno())


def checked_connection(path: Path, immutable: bool = False) -> sqlite3.Connection:
    regular_file(path)
    options = "?mode=ro" + ("&immutable=1" if immutable else "")
    connection = sqlite3.connect(path.absolute().as_uri() + options, uri=True, timeout=10)
    connection.execute("PRAGMA query_only=ON")
    return connection


def verify(path: Path) -> None:
    # Only closed, standalone copies are verified here, never the live source.
    with closing(checked_connection(path, immutable=True)) as connection:
        integrity = connection.execute("PRAGMA integrity_check").fetchall()
        if integrity != [("ok",)]:
            raise ValueError("SQLite integrity_check failed")
        if connection.execute("PRAGMA foreign_key_check").fetchone() is not None:
            raise ValueError("SQLite foreign_key_check failed")


def copy_database(source: Path, destination: Path, source_immutable: bool = False) -> None:
    descriptor = os.open(destination, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(descriptor)
    deadline = time.monotonic() + 60

    def progress(_status: int, _remaining: int, _total: int) -> None:
        if time.monotonic() > deadline:
            raise TimeoutError("SQLite backup exceeded its 60 second budget")

    try:
        with closing(checked_connection(source, immutable=source_immutable)) as reader, closing(sqlite3.connect(destination)) as writer:
            reader.backup(writer, pages=128, progress=progress, sleep=0.05)
            writer.commit()
            # Normalize this copy only. A WAL-mode header in a detached backup
            # can create root-owned -wal/-shm files during verification/restore,
            # preventing the runtime user from opening its restored database.
            if writer.execute("PRAGMA journal_mode=DELETE").fetchone() != ("delete",):
                raise ValueError("could not make the snapshot standalone")
        if any(Path(str(destination) + suffix).exists() for suffix in ("-wal", "-shm", "-journal")):
            raise ValueError("snapshot retained journal sidecars")
        verify(destination)
        with destination.open("r+b") as stream:
            os.fsync(stream.fileno())
        sync_directory(destination.parent)
    except BaseException:
        # This function created the file exclusively; never remove the source.
        destination.unlink(missing_ok=True)
        for suffix in ("-wal", "-shm", "-journal"):
            Path(str(destination) + suffix).unlink(missing_ok=True)
        raise


def verify_checksum(path: Path) -> str:
    regular_file(path)
    checksum_path = Path(str(path) + ".sha256")
    regular_file(checksum_path)
    if checksum_path.stat().st_size > 512:
        raise ValueError("oversized checksum receipt")
    expected = checksum_path.read_text(encoding="utf-8").strip()
    actual = digest(path)
    if expected != actual:
        raise ValueError("backup checksum mismatch")
    return actual


def backup(database: Path, directory: Path, keep: int) -> dict:
    if not 1 <= keep <= 365:
        raise ValueError("retention must be between 1 and 365 complete backups")
    regular_file(database)
    private_directory(directory)
    name = "control-" + datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ-") + uuid.uuid4().hex[:8] + ".db"
    temporary = directory / (".partial-" + uuid.uuid4().hex + ".db")
    target = directory / name
    try:
        copy_database(database, temporary)
        actual = digest(temporary)
        # A hard link publishes the complete file atomically and cannot replace
        # an existing target, including a concurrently created file or symlink.
        os.link(temporary, target)
        exclusive_text(Path(str(target) + ".sha256"), actual + "\n")
        sync_directory(directory)
    finally:
        temporary.unlink(missing_ok=True)
    complete = []
    for candidate in directory.glob("control-*.db"):
        if not BACKUP_NAME.fullmatch(candidate.name) or candidate.is_symlink():
            continue
        try:
            verify_checksum(candidate)
        except (ValueError, OSError):
            continue  # Unrelated, incomplete or altered files are preserved.
        complete.append(candidate)
    removed = 0
    for expired in sorted(complete, key=lambda path: path.stat().st_mtime_ns, reverse=True)[keep:]:
        expired.unlink()
        Path(str(expired) + ".sha256").unlink()
        removed += 1
    sync_directory(directory)
    return {"operation": "backup", "path": str(target), "sha256": actual, "integrity": "ok", "pruned_owned_backups": removed}


def restore(source: Path, destination: Path) -> dict:
    actual = verify_checksum(source)
    private_directory(destination.parent)
    if destination.exists() or destination.is_symlink():
        raise FileExistsError("restore destination already exists; refusing overwrite")
    copy_database(source, destination, source_immutable=True)
    return {"operation": "restore", "source_sha256": actual, "path": str(destination), "integrity": "ok", "overwrote_existing": False}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="operation", required=True)
    save = subparsers.add_parser("backup")
    save.add_argument("--database", type=Path, required=True)
    save.add_argument("--directory", type=Path, required=True)
    save.add_argument("--keep", type=int, default=14)
    load = subparsers.add_parser("restore")
    load.add_argument("--backup", type=Path, required=True)
    load.add_argument("--destination", type=Path, required=True)
    args = parser.parse_args()
    if args.operation == "backup":
        result = backup(args.database, args.directory, args.keep)
    else:
        result = restore(args.backup, args.destination)
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
