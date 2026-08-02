export namespace main {
	
	export class BackendStatus {
	    goStarted: boolean;
	    goBaseURL: string;
	
	    static createFrom(source: any = {}) {
	        return new BackendStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goStarted = source["goStarted"];
	        this.goBaseURL = source["goBaseURL"];
	    }
	}
	export class WindowState {
	    maximised: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WindowState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maximised = source["maximised"];
	    }
	}

}

