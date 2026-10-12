export namespace api {
	
	export class AppInfo {
	    name: string;
	    shortName: string;
	    version: string;
	    dataDir: string;
	    portable: boolean;
	    os: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.shortName = source["shortName"];
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	        this.portable = source["portable"];
	        this.os = source["os"];
	    }
	}

}

export namespace settings {
	
	export class CustomCSS {
	    enabled: boolean;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomCSS(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.path = source["path"];
	    }
	}
	export class Appearance {
	    theme: string;
	    accent: string;
	    density: string;
	    textSize: string;
	    reduceMotion: string;
	    customCss: CustomCSS;
	
	    static createFrom(source: any = {}) {
	        return new Appearance(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.accent = source["accent"];
	        this.density = source["density"];
	        this.textSize = source["textSize"];
	        this.reduceMotion = source["reduceMotion"];
	        this.customCss = this.convertValues(source["customCss"], CustomCSS);
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
	
	export class Downloads {
	    parallel: number;
	    watchDownloads: boolean;
	    proxy: string;
	
	    static createFrom(source: any = {}) {
	        return new Downloads(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.parallel = source["parallel"];
	        this.watchDownloads = source["watchDownloads"];
	        this.proxy = source["proxy"];
	    }
	}
	export class General {
	    language: string;
	    onGameStart: string;
	    startPage: string;
	    startWithComputer: boolean;
	
	    static createFrom(source: any = {}) {
	        return new General(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.onGameStart = source["onGameStart"];
	        this.startPage = source["startPage"];
	        this.startWithComputer = source["startWithComputer"];
	    }
	}
	export class Integrations {
	    discordStatus: boolean;
	    handleLinks: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Integrations(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.discordStatus = source["discordStatus"];
	        this.handleLinks = source["handleLinks"];
	    }
	}
	export class Java {
	    autoPick: boolean;
	    downloadVendor: string;
	    memoryMinMb: number;
	    memoryMaxMb: number;
	    jvmArgs: string;
	    warnLowMemory: boolean;
	    suggestMemory: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Java(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoPick = source["autoPick"];
	        this.downloadVendor = source["downloadVendor"];
	        this.memoryMinMb = source["memoryMinMb"];
	        this.memoryMaxMb = source["memoryMaxMb"];
	        this.jvmArgs = source["jvmArgs"];
	        this.warnLowMemory = source["warnLowMemory"];
	        this.suggestMemory = source["suggestMemory"];
	    }
	}
	export class Updates {
	    autoCheck: boolean;
	    modChannel: string;
	
	    static createFrom(source: any = {}) {
	        return new Updates(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoCheck = source["autoCheck"];
	        this.modChannel = source["modChannel"];
	    }
	}
	export class Settings {
	    version: number;
	    general: General;
	    downloads: Downloads;
	    integrations: Integrations;
	    appearance: Appearance;
	    java: Java;
	    updates: Updates;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.general = this.convertValues(source["general"], General);
	        this.downloads = this.convertValues(source["downloads"], Downloads);
	        this.integrations = this.convertValues(source["integrations"], Integrations);
	        this.appearance = this.convertValues(source["appearance"], Appearance);
	        this.java = this.convertValues(source["java"], Java);
	        this.updates = this.convertValues(source["updates"], Updates);
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

}

export namespace theme {
	
	export class CSSState {
	    path: string;
	    css: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new CSSState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.css = source["css"];
	        this.error = source["error"];
	    }
	}

}

