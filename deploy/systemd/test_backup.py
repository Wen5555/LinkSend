import importlib.util
from contextlib import closing
from pathlib import Path
import sqlite3
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("linksend_backup", Path(__file__).with_name("linksend-backup.py"))
backup_tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup_tool)


class BackupTests(unittest.TestCase):
    def test_online_wal_snapshot_and_no_overwrite_restore(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            database = root / "live.db"
            with closing(sqlite3.connect(database)) as live:
                live.execute("PRAGMA journal_mode=WAL")
                live.execute("CREATE TABLE events(id INTEGER PRIMARY KEY, data TEXT)")
                live.execute("INSERT INTO events(data) VALUES('committed')")
                live.commit()
                receipt = backup_tool.backup(database, root / "backups", 2)
                live.execute("INSERT INTO events(data) VALUES('later')")
                live.commit()
                restored = root / "isolated" / "control.db"
                backup_tool.restore(Path(receipt["path"]), restored)
                with closing(sqlite3.connect(restored)) as copy:
                    self.assertEqual(copy.execute("SELECT data FROM events").fetchall(), [("committed",)])
                with self.assertRaises(FileExistsError):
                    backup_tool.restore(Path(receipt["path"]), restored)
                self.assertEqual(live.execute("SELECT count(*) FROM events").fetchone()[0], 2)

    def test_checksum_failure_and_retention_preserve_unowned_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            database = root / "live.db"
            with closing(sqlite3.connect(database)) as connection:
                connection.execute("CREATE TABLE sample(id INTEGER)")
                connection.commit()
            directory = root / "backups"
            first = backup_tool.backup(database, directory, 2)
            unowned = directory / "control-user-data.db"
            unowned.write_bytes(b"preserve")
            incomplete = directory / "control-20000101T000000Z-00000000.db"
            incomplete.write_bytes(b"not a completed backup")
            second = backup_tool.backup(database, directory, 1)
            self.assertFalse(Path(first["path"]).exists())
            self.assertTrue(unowned.exists())
            self.assertTrue(incomplete.exists())
            Path(second["path"]).write_bytes(b"changed")
            with self.assertRaises(ValueError):
                backup_tool.restore(Path(second["path"]), root / "restore" / "db")
            self.assertFalse((root / "restore" / "db").exists())


if __name__ == "__main__":
    unittest.main()
