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

}

export namespace main {
	
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
