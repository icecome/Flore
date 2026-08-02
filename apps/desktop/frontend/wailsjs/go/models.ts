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

export namespace updater {
	
	export class UpdateInfo {
	    currentVersion: string;
	    latestVersion: string;
	    notes: string;
	    size: number;
	    fileName: string;
	    sha256: string;
	    urls: string[];
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.notes = source["notes"];
	        this.size = source["size"];
	        this.fileName = source["fileName"];
	        this.sha256 = source["sha256"];
	        this.urls = source["urls"];
	    }
	}

}

