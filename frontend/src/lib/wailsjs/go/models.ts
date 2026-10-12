export namespace accounts {
	
	export class SkinColors {
	    skin: string;
	    hair: string;
	    eye: string;
	    mouth: string;
	    top: string;
	    topD: string;
	    pants: string;
	    shoes: string;
	
	    static createFrom(source: any = {}) {
	        return new SkinColors(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.skin = source["skin"];
	        this.hair = source["hair"];
	        this.eye = source["eye"];
	        this.mouth = source["mouth"];
	        this.top = source["top"];
	        this.topD = source["topD"];
	        this.pants = source["pants"];
	        this.shoes = source["shoes"];
	    }
	}

}

export namespace api {
	
	export class AccountView {
	    id: string;
	    name: string;
	    kind: string;
	    status: string;
	    detail: string;
	    isDefault: boolean;
	    skin: accounts.SkinColors;
	    skinHash: string;
	    slim: boolean;
	    refreshedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new AccountView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.detail = source["detail"];
	        this.isDefault = source["isDefault"];
	        this.skin = this.convertValues(source["skin"], accounts.SkinColors);
	        this.skinHash = source["skinHash"];
	        this.slim = source["slim"];
	        this.refreshedAt = source["refreshedAt"];
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
	export class AppInfo {
	    name: string;
	    shortName: string;
	    version: string;
	    dataDir: string;
	    portable: boolean;
	    os: string;
	    memoryMb: number;
	
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
	        this.memoryMb = source["memoryMb"];
	    }
	}
	export class CapeView {
	    id: string;
	    name: string;
	    texture: string;
	
	    static createFrom(source: any = {}) {
	        return new CapeView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.texture = source["texture"];
	    }
	}
	export class CapesView {
	    capes: CapeView[];
	    worn: string;
	
	    static createFrom(source: any = {}) {
	        return new CapesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capes = this.convertValues(source["capes"], CapeView);
	        this.worn = source["worn"];
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
	export class InstanceSummary {
	    id: string;
	    name: string;
	    art: instance.Art;
	    minecraft: string;
	    kind: string;
	    hasServers: boolean;
	    lastPlayed?: number;
	    playtimeMin: number;
	    running: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InstanceSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.art = this.convertValues(source["art"], instance.Art);
	        this.minecraft = source["minecraft"];
	        this.kind = source["kind"];
	        this.hasServers = source["hasServers"];
	        this.lastPlayed = source["lastPlayed"];
	        this.playtimeMin = source["playtimeMin"];
	        this.running = source["running"];
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
	export class MemoryRange {
	    min: number;
	    max: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryRange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.min = source["min"];
	        this.max = source["max"];
	    }
	}
	export class InstanceView {
	    id: string;
	    name: string;
	    art: instance.Art;
	    minecraft: string;
	    kind: string;
	    hasServers: boolean;
	    lastPlayed?: number;
	    playtimeMin: number;
	    running: boolean;
	    sandboxed: boolean;
	    counts: Record<string, number>;
	    java: string;
	    memoryMB: MemoryRange;
	    created: number;
	    folder: string;
	    syncTargets: number;
	    discordStatus: boolean;
	    settings: instance.Settings;
	
	    static createFrom(source: any = {}) {
	        return new InstanceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.art = this.convertValues(source["art"], instance.Art);
	        this.minecraft = source["minecraft"];
	        this.kind = source["kind"];
	        this.hasServers = source["hasServers"];
	        this.lastPlayed = source["lastPlayed"];
	        this.playtimeMin = source["playtimeMin"];
	        this.running = source["running"];
	        this.sandboxed = source["sandboxed"];
	        this.counts = source["counts"];
	        this.java = source["java"];
	        this.memoryMB = this.convertValues(source["memoryMB"], MemoryRange);
	        this.created = source["created"];
	        this.folder = source["folder"];
	        this.syncTargets = source["syncTargets"];
	        this.discordStatus = source["discordStatus"];
	        this.settings = this.convertValues(source["settings"], instance.Settings);
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
	export class JavaRuntime {
	    major: number;
	    name: string;
	    source: string;
	    path: string;
	    usedBy: number;
	
	    static createFrom(source: any = {}) {
	        return new JavaRuntime(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.major = source["major"];
	        this.name = source["name"];
	        this.source = source["source"];
	        this.path = source["path"];
	        this.usedBy = source["usedBy"];
	    }
	}
	
	export class SkinView {
	    hash: string;
	    name: string;
	    slim: boolean;
	    added: number;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new SkinView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hash = source["hash"];
	        this.name = source["name"];
	        this.slim = source["slim"];
	        this.added = source["added"];
	        this.source = source["source"];
	    }
	}

}

export namespace gamelog {
	
	export class Line {
	    time: string;
	    level: string;
	    thread: string;
	    message: string;
	    redacted?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Line(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.thread = source["thread"];
	        this.message = source["message"];
	        this.redacted = source["redacted"];
	    }
	}

}

export namespace instance {
	
	export class Art {
	    background: string;
	    initial?: string;
	
	    static createFrom(source: any = {}) {
	        return new Art(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.background = source["background"];
	        this.initial = source["initial"];
	    }
	}
	export class CreateRequest {
	    name: string;
	    art: Art;
	    minecraft: string;
	    loader: string;
	    group: string;
	    sandbox: boolean;
	    shares: string[];
	
	    static createFrom(source: any = {}) {
	        return new CreateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.art = this.convertValues(source["art"], Art);
	        this.minecraft = source["minecraft"];
	        this.loader = source["loader"];
	        this.group = source["group"];
	        this.sandbox = source["sandbox"];
	        this.shares = source["shares"];
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
	export class JavaSettings {
	    override: boolean;
	    javaPath: string;
	    memoryMinMb: number;
	    memoryMaxMb: number;
	    jvmArgs: string;
	
	    static createFrom(source: any = {}) {
	        return new JavaSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.override = source["override"];
	        this.javaPath = source["javaPath"];
	        this.memoryMinMb = source["memoryMinMb"];
	        this.memoryMaxMb = source["memoryMaxMb"];
	        this.jvmArgs = source["jvmArgs"];
	    }
	}
	export class WindowSettings {
	    width: number;
	    height: number;
	    fullscreen: boolean;
	    onGameStart: string;
	
	    static createFrom(source: any = {}) {
	        return new WindowSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fullscreen = source["fullscreen"];
	        this.onGameStart = source["onGameStart"];
	    }
	}
	export class Settings {
	    java: JavaSettings;
	    window: WindowSettings;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.java = this.convertValues(source["java"], JavaSettings);
	        this.window = this.convertValues(source["window"], WindowSettings);
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

export namespace launch {
	
	export class MemoryRange {
	    min: number;
	    max: number;
	
	    static createFrom(source: any = {}) {
	        return new MemoryRange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.min = source["min"];
	        this.max = source["max"];
	    }
	}
	export class Record {
	    id: string;
	    startedAt: number;
	    lengthMin: number;
	    exitCode: number;
	    result: string;
	    java: string;
	    memory: MemoryRange;
	    account: string;
	    accountId: string;
	
	    static createFrom(source: any = {}) {
	        return new Record(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.startedAt = source["startedAt"];
	        this.lengthMin = source["lengthMin"];
	        this.exitCode = source["exitCode"];
	        this.result = source["result"];
	        this.java = source["java"];
	        this.memory = this.convertValues(source["memory"], MemoryRange);
	        this.account = source["account"];
	        this.accountId = source["accountId"];
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
	export class Status {
	    running: string[];
	    launching: string[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.launching = source["launching"];
	    }
	}

}

export namespace mojang {
	
	export class Release {
	    id: string;
	    java: number;
	    released: number;
	    snapshot?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.java = source["java"];
	        this.released = source["released"];
	        this.snapshot = source["snapshot"];
	    }
	}
	export class Family {
	    id: string;
	    note: string;
	    releases: Release[];
	    snapshots: Release[];
	
	    static createFrom(source: any = {}) {
	        return new Family(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.note = source["note"];
	        this.releases = this.convertValues(source["releases"], Release);
	        this.snapshots = this.convertValues(source["snapshots"], Release);
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

export namespace settings {
	
	export class Accounts {
	    autoRefresh: boolean;
	    offlineFallback: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Accounts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.autoRefresh = source["autoRefresh"];
	        this.offlineFallback = source["offlineFallback"];
	    }
	}
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
	
	export class ProxyCustom {
	    type: string;
	    host: string;
	    port: number;
	    user: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyCustom(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	    }
	}
	export class Downloads {
	    parallel: number;
	    watchDownloads: boolean;
	    proxy: string;
	    proxyCustom: ProxyCustom;
	
	    static createFrom(source: any = {}) {
	        return new Downloads(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.parallel = source["parallel"];
	        this.watchDownloads = source["watchDownloads"];
	        this.proxy = source["proxy"];
	        this.proxyCustom = this.convertValues(source["proxyCustom"], ProxyCustom);
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
	    accounts: Accounts;
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
	        this.accounts = this.convertValues(source["accounts"], Accounts);
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

export namespace tasks {
	
	export class Art {
	    background: string;
	    initial?: string;
	
	    static createFrom(source: any = {}) {
	        return new Art(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.background = source["background"];
	        this.initial = source["initial"];
	    }
	}
	export class StepView {
	    name: string;
	    unit: string;
	    done: number;
	    total: number;
	    state: string;
	
	    static createFrom(source: any = {}) {
	        return new StepView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.unit = source["unit"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.state = source["state"];
	    }
	}
	export class Meta {
	    title: string;
	    subtitle: string;
	    art: Art;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new Meta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.subtitle = source["subtitle"];
	        this.art = this.convertValues(source["art"], Art);
	        this.kind = source["kind"];
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
	export class JobView {
	    id: string;
	    meta: Meta;
	    state: string;
	    doneBytes: number;
	    totalBytes: number;
	    speed: number;
	    etaSec: number;
	    steps: StepView[];
	    error?: string;
	    retryable: boolean;
	    createdAt: number;
	    finishedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new JobView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.meta = this.convertValues(source["meta"], Meta);
	        this.state = source["state"];
	        this.doneBytes = source["doneBytes"];
	        this.totalBytes = source["totalBytes"];
	        this.speed = source["speed"];
	        this.etaSec = source["etaSec"];
	        this.steps = this.convertValues(source["steps"], StepView);
	        this.error = source["error"];
	        this.retryable = source["retryable"];
	        this.createdAt = source["createdAt"];
	        this.finishedAt = source["finishedAt"];
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
	
	export class Snapshot {
	    active: JobView[];
	    speed: number;
	    etaSec: number;
	    historyRev: number;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = this.convertValues(source["active"], JobView);
	        this.speed = source["speed"];
	        this.etaSec = source["etaSec"];
	        this.historyRev = source["historyRev"];
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

