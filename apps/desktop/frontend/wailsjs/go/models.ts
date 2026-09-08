export namespace app {

	export class DeviceInfo {
	    id: string;
	    group_id: string;
	    name: string;
	    public_key_hex: string;
	    admin: boolean;
	    online: boolean;
	    trusted: boolean;

	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.group_id = source["group_id"];
	        this.name = source["name"];
	        this.public_key_hex = source["public_key_hex"];
	        this.admin = source["admin"];
	        this.online = source["online"];
	        this.trusted = source["trusted"];
	    }
	}
	export class DiagnosticIdentity {
	    id: string;
	    public_key_hex: string;

	    static createFrom(source: any = {}) {
	        return new DiagnosticIdentity(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.public_key_hex = source["public_key_hex"];
	    }
	}
	export class Diagnostics {
	    version: string;
	    platform: string;
	    relay: boolean;
	    identity: DiagnosticIdentity;
	    server_url?: string;
	    server_health: string;
	    capabilities: protocol.Capabilities;
	    trusted_peers: number;
	    generated_at: string;
	    health_failure?: string;

	    static createFrom(source: any = {}) {
	        return new Diagnostics(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.platform = source["platform"];
	        this.relay = source["relay"];
	        this.identity = this.convertValues(source["identity"], DiagnosticIdentity);
	        this.server_url = source["server_url"];
	        this.server_health = source["server_health"];
	        this.capabilities = this.convertValues(source["capabilities"], protocol.Capabilities);
	        this.trusted_peers = source["trusted_peers"];
	        this.generated_at = source["generated_at"];
	        this.health_failure = source["health_failure"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IdentityInfo {
	    id: string;
	    public_key_hex: string;
	    data_dir: string;

	    static createFrom(source: any = {}) {
	        return new IdentityInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.public_key_hex = source["public_key_hex"];
	        this.data_dir = source["data_dir"];
	    }
	}
	export class InvitationInfo {
	    token: string;
	    expires_at: string;

	    static createFrom(source: any = {}) {
	        return new InvitationInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class TaskSnapshot {
	    id: string;
	    direction: string;
	    peer_id?: string;
	    source_summary?: string;
	    manifest_summary?: string;
	    file_count?: number;
	    target_directory?: string;
	    state: string;
	    phase: string;
	    processed_bytes: number;
	    total_bytes?: number;
	    rate_bytes_per_second?: number;
	    started_at: string;
	    updated_at: string;
	    ended_at?: string;
	    error_code?: string;
	    error_message?: string;
	    transfer_id?: string;
	    session_id?: string;
	    can_cancel: boolean;
	    can_retry: boolean;

	    static createFrom(source: any = {}) {
	        return new TaskSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.direction = source["direction"];
	        this.peer_id = source["peer_id"];
	        this.source_summary = source["source_summary"];
	        this.manifest_summary = source["manifest_summary"];
	        this.file_count = source["file_count"];
	        this.target_directory = source["target_directory"];
	        this.state = source["state"];
	        this.phase = source["phase"];
	        this.processed_bytes = source["processed_bytes"];
	        this.total_bytes = source["total_bytes"];
	        this.rate_bytes_per_second = source["rate_bytes_per_second"];
	        this.started_at = source["started_at"];
	        this.updated_at = source["updated_at"];
	        this.ended_at = source["ended_at"];
	        this.error_code = source["error_code"];
	        this.error_message = source["error_message"];
	        this.transfer_id = source["transfer_id"];
	        this.session_id = source["session_id"];
	        this.can_cancel = source["can_cancel"];
	        this.can_retry = source["can_retry"];
	    }
	}

}

export namespace main {

	export class DesktopPreferences {
	    format_version: number;
	    server_url: string;
	    bind_address: string;
	    stun_urls: string[];
	    receive_directory: string;
	    device_name: string;

	    static createFrom(source: any = {}) {
	        return new DesktopPreferences(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format_version = source["format_version"];
	        this.server_url = source["server_url"];
	        this.bind_address = source["bind_address"];
	        this.stun_urls = source["stun_urls"];
	        this.receive_directory = source["receive_directory"];
	        this.device_name = source["device_name"];
	    }
	}
	export class DesktopStatus {
	    version: string;
	    platform: string;
	    relay: boolean;
	    stage: string;
	    ready: boolean;
	    error?: string;
	    identity?: string;

	    static createFrom(source: any = {}) {
	        return new DesktopStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.platform = source["platform"];
	        this.relay = source["relay"];
	        this.stage = source["stage"];
	        this.ready = source["ready"];
	        this.error = source["error"];
	        this.identity = source["identity"];
	    }
	}
	export class NetworkInterfaceInfo {
	    name: string;
	    addresses: string[];
	    is_loopback: boolean;

	    static createFrom(source: any = {}) {
	        return new NetworkInterfaceInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.addresses = source["addresses"];
	        this.is_loopback = source["is_loopback"];
	    }
	}

}

export namespace protocol {

	export class Capabilities {
	    protocol_version: number;
	    relay: boolean;
	    transport: string;

	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol_version = source["protocol_version"];
	        this.relay = source["relay"];
	        this.transport = source["transport"];
	    }
	}

}
