export namespace main {
	
	export class AuthResponse {
	    success: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	    }
	}
	export class MetricsData {
	    cpu_usage: number;
	    ram_usage: number;
	    disk_usage: number;
	    network_tx: number;
	    network_rx: number;
	
	    static createFrom(source: any = {}) {
	        return new MetricsData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cpu_usage = source["cpu_usage"];
	        this.ram_usage = source["ram_usage"];
	        this.disk_usage = source["disk_usage"];
	        this.network_tx = source["network_tx"];
	        this.network_rx = source["network_rx"];
	    }
	}

}

