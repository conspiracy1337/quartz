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
	export class ImportPreview {
	    token: string;
	    name: string;
	    version: string;
	    minecraft: string;
	    loader: string;
	    source: string;
	    icon?: string;
	    stats: modpack.Stats;
	    warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportPreview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.name = source["name"];
	        this.version = source["version"];
	        this.minecraft = source["minecraft"];
	        this.loader = source["loader"];
	        this.source = source["source"];
	        this.icon = source["icon"];
	        this.stats = this.convertValues(source["stats"], modpack.Stats);
	        this.warnings = source["warnings"];
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
	export class LoaderRef {
	    kind: string;
	    version: string;
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new LoaderRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.version = source["version"];
	        this.id = source["id"];
	    }
	}
	export class InstanceSummary {
	    id: string;
	    name: string;
	    art: instance.Art;
	    icon?: string;
	    minecraft: string;
	    loader?: LoaderRef;
	    kind: string;
	    group?: string;
	    tags: string[];
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
	        this.icon = source["icon"];
	        this.minecraft = source["minecraft"];
	        this.loader = this.convertValues(source["loader"], LoaderRef);
	        this.kind = source["kind"];
	        this.group = source["group"];
	        this.tags = source["tags"];
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
	    icon?: string;
	    minecraft: string;
	    loader?: LoaderRef;
	    kind: string;
	    group?: string;
	    tags: string[];
	    hasServers: boolean;
	    lastPlayed?: number;
	    playtimeMin: number;
	    running: boolean;
	    sandboxed: boolean;
	    counts: Record<string, number>;
	    java: string;
	    javaAuto: string;
	    memoryMB: MemoryRange;
	    created: number;
	    folder: string;
	    moved: boolean;
	    notes: string;
	    keepLoaderUpdated: boolean;
	    followLatest: boolean;
	    syncTargets: number;
	    discordStatus: boolean;
	    settings: instance.Settings;
	    gpu: string;
	    switch?: content.SwitchResult;
	
	    static createFrom(source: any = {}) {
	        return new InstanceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.art = this.convertValues(source["art"], instance.Art);
	        this.icon = source["icon"];
	        this.minecraft = source["minecraft"];
	        this.loader = this.convertValues(source["loader"], LoaderRef);
	        this.kind = source["kind"];
	        this.group = source["group"];
	        this.tags = source["tags"];
	        this.hasServers = source["hasServers"];
	        this.lastPlayed = source["lastPlayed"];
	        this.playtimeMin = source["playtimeMin"];
	        this.running = source["running"];
	        this.sandboxed = source["sandboxed"];
	        this.counts = source["counts"];
	        this.java = source["java"];
	        this.javaAuto = source["javaAuto"];
	        this.memoryMB = this.convertValues(source["memoryMB"], MemoryRange);
	        this.created = source["created"];
	        this.folder = source["folder"];
	        this.moved = source["moved"];
	        this.notes = source["notes"];
	        this.keepLoaderUpdated = source["keepLoaderUpdated"];
	        this.followLatest = source["followLatest"];
	        this.syncTargets = source["syncTargets"];
	        this.discordStatus = source["discordStatus"];
	        this.settings = this.convertValues(source["settings"], instance.Settings);
	        this.gpu = source["gpu"];
	        this.switch = this.convertValues(source["switch"], content.SwitchResult);
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
	export class LoaderVersion {
	    id: string;
	    name: string;
	    released: number;
	
	    static createFrom(source: any = {}) {
	        return new LoaderVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.released = source["released"];
	    }
	}
	export class LoaderOption {
	    kind: string;
	    name: string;
	    note: string;
	    stable: LoaderVersion[];
	    beta: LoaderVersion[];
	
	    static createFrom(source: any = {}) {
	        return new LoaderOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.note = source["note"];
	        this.stable = this.convertValues(source["stable"], LoaderVersion);
	        this.beta = this.convertValues(source["beta"], LoaderVersion);
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

export namespace browse {
	
	export class InstalledVersion {
	    item: string;
	    source: string;
	    versionId: string;
	    version: string;
	    published: number;
	
	    static createFrom(source: any = {}) {
	        return new InstalledVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.item = source["item"];
	        this.source = source["source"];
	        this.versionId = source["versionId"];
	        this.version = source["version"];
	        this.published = source["published"];
	    }
	}
	export class Link {
	    icon: string;
	    label: string;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new Link(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.icon = source["icon"];
	        this.label = source["label"];
	        this.url = source["url"];
	    }
	}
	export class Notes {
	    id: string;
	    number: string;
	    published: number;
	    items: string[];
	
	    static createFrom(source: any = {}) {
	        return new Notes(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.number = source["number"];
	        this.published = source["published"];
	        this.items = source["items"];
	    }
	}
	export class Picture {
	    src: string;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new Picture(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.src = source["src"];
	        this.title = source["title"];
	    }
	}
	export class Side {
	    name: string;
	    author: string;
	    summary: string;
	    icon?: string;
	    downloads: number;
	    updated: number;
	    url: string;
	    versions: number;
	    badges: string[];
	    body: docs.Block[];
	    gallery: Picture[];
	    links: Link[];
	    blocked?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Side(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.author = source["author"];
	        this.summary = source["summary"];
	        this.icon = source["icon"];
	        this.downloads = source["downloads"];
	        this.updated = source["updated"];
	        this.url = source["url"];
	        this.versions = source["versions"];
	        this.badges = source["badges"];
	        this.body = this.convertValues(source["body"], docs.Block);
	        this.gallery = this.convertValues(source["gallery"], Picture);
	        this.links = this.convertValues(source["links"], Link);
	        this.blocked = source["blocked"];
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
	export class Page {
	    ref: string;
	    type: string;
	    name: string;
	    modrinth?: Side;
	    curseforge?: Side;
	    fresher: string;
	    installed?: InstalledVersion;
	
	    static createFrom(source: any = {}) {
	        return new Page(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ref = source["ref"];
	        this.type = source["type"];
	        this.name = source["name"];
	        this.modrinth = this.convertValues(source["modrinth"], Side);
	        this.curseforge = this.convertValues(source["curseforge"], Side);
	        this.fresher = source["fresher"];
	        this.installed = this.convertValues(source["installed"], InstalledVersion);
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
	
	export class Query {
	    type: string;
	    text: string;
	    sort: string;
	    instance: string;
	    version: boolean;
	    loader: boolean;
	    categories: string[];
	    hideAi: boolean;
	    hideAds: boolean;
	    hideInstalled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Query(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.text = source["text"];
	        this.sort = source["sort"];
	        this.instance = source["instance"];
	        this.version = source["version"];
	        this.loader = source["loader"];
	        this.categories = source["categories"];
	        this.hideAi = source["hideAi"];
	        this.hideAds = source["hideAds"];
	        this.hideInstalled = source["hideInstalled"];
	    }
	}
	export class Stats {
	    downloads: number;
	    updated: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.downloads = source["downloads"];
	        this.updated = source["updated"];
	    }
	}
	export class Row {
	    id: string;
	    ref: string;
	    name: string;
	    author: string;
	    summary: string;
	    icon?: string;
	    modrinth?: Stats;
	    curseforge?: Stats;
	
	    static createFrom(source: any = {}) {
	        return new Row(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ref = source["ref"];
	        this.name = source["name"];
	        this.author = source["author"];
	        this.summary = source["summary"];
	        this.icon = source["icon"];
	        this.modrinth = this.convertValues(source["modrinth"], Stats);
	        this.curseforge = this.convertValues(source["curseforge"], Stats);
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
	export class Results {
	    searchId: string;
	    rows: Row[];
	    changed: Row[];
	    more: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Results(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.searchId = source["searchId"];
	        this.rows = this.convertValues(source["rows"], Row);
	        this.changed = this.convertValues(source["changed"], Row);
	        this.more = source["more"];
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
	
	
	
	export class VersionRow {
	    id: string;
	    number: string;
	    channel: string;
	    minecraft: string;
	    published: number;
	    bytes: number;
	    action: string;
	
	    static createFrom(source: any = {}) {
	        return new VersionRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.number = source["number"];
	        this.channel = source["channel"];
	        this.minecraft = source["minecraft"];
	        this.published = source["published"];
	        this.bytes = source["bytes"];
	        this.action = source["action"];
	    }
	}

}

export namespace content {
	
	export class Activity {
	    at: number;
	    kind: string;
	    title: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new Activity(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = source["at"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	    }
	}
	export class Rejected {
	    name: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Rejected(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.reason = source["reason"];
	    }
	}
	export class AddResult {
	    added: string[];
	    rejected: Rejected[];
	
	    static createFrom(source: any = {}) {
	        return new AddResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.added = source["added"];
	        this.rejected = this.convertValues(source["rejected"], Rejected);
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
	export class Compat {
	    matching: number;
	    total: number;
	    unknown: number;
	
	    static createFrom(source: any = {}) {
	        return new Compat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matching = source["matching"];
	        this.total = source["total"];
	        this.unknown = source["unknown"];
	    }
	}
	export class InstallDep {
	    id: string;
	    name: string;
	    summary?: string;
	    icon?: string;
	    required: boolean;
	    installed: boolean;
	    size: number;
	    blocked?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InstallDep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.summary = source["summary"];
	        this.icon = source["icon"];
	        this.required = source["required"];
	        this.installed = source["installed"];
	        this.size = source["size"];
	        this.blocked = source["blocked"];
	    }
	}
	export class InstallPlan {
	    planId: string;
	    name: string;
	    kind: string;
	    version: string;
	    size: number;
	    replaces?: string;
	    blocked?: boolean;
	    deps: InstallDep[];
	    mods: number;
	
	    static createFrom(source: any = {}) {
	        return new InstallPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.planId = source["planId"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.version = source["version"];
	        this.size = source["size"];
	        this.replaces = source["replaces"];
	        this.blocked = source["blocked"];
	        this.deps = this.convertValues(source["deps"], InstallDep);
	        this.mods = source["mods"];
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
	export class InstanceMissing {
	    instanceId: string;
	    mod: string;
	    dep: string;
	    more?: number;
	
	    static createFrom(source: any = {}) {
	        return new InstanceMissing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instanceId = source["instanceId"];
	        this.mod = source["mod"];
	        this.dep = source["dep"];
	        this.more = source["more"];
	    }
	}
	export class LockedItem {
	    id: string;
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new LockedItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}
	export class Notes {
	    id: string;
	    date: number;
	    items: string[];
	
	    static createFrom(source: any = {}) {
	        return new Notes(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.date = source["date"];
	        this.items = source["items"];
	    }
	}
	export class PendingUpdate {
	    item: string;
	    kind: string;
	    name: string;
	    icon?: string;
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new PendingUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.item = source["item"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	
	export class ReviewItem {
	    id: string;
	    name: string;
	    note: string;
	    from: string;
	    to: string;
	    icon?: string;
	    size: number;
	    blocked?: boolean;
	    dependency?: boolean;
	    versions?: string;
	    ref?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReviewItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.note = source["note"];
	        this.from = source["from"];
	        this.to = source["to"];
	        this.icon = source["icon"];
	        this.size = source["size"];
	        this.blocked = source["blocked"];
	        this.dependency = source["dependency"];
	        this.versions = source["versions"];
	        this.ref = source["ref"];
	    }
	}
	export class Review {
	    checkId: string;
	    updates: ReviewItem[];
	    locked: LockedItem[];
	    failed: string[];
	
	    static createFrom(source: any = {}) {
	        return new Review(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checkId = source["checkId"];
	        this.updates = this.convertValues(source["updates"], ReviewItem);
	        this.locked = this.convertValues(source["locked"], LockedItem);
	        this.failed = source["failed"];
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
	    kind: string;
	    version?: string;
	    label?: string;
	    tone?: string;
	    world?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.version = source["version"];
	        this.label = source["label"];
	        this.tone = source["tone"];
	        this.world = source["world"];
	    }
	}
	export class Row {
	    id: string;
	    kind: string;
	    name: string;
	    sub: string;
	    version: string;
	    icon?: string;
	    source: string;
	    enabled: boolean;
	    locked: boolean;
	    group?: string;
	    status?: Status;
	    project?: string;
	    ref?: string;
	
	    static createFrom(source: any = {}) {
	        return new Row(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.sub = source["sub"];
	        this.version = source["version"];
	        this.icon = source["icon"];
	        this.source = source["source"];
	        this.enabled = source["enabled"];
	        this.locked = source["locked"];
	        this.group = source["group"];
	        this.status = this.convertValues(source["status"], Status);
	        this.project = source["project"];
	        this.ref = source["ref"];
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
	
	export class SwitchResult {
	    job: string;
	    updated: number;
	    disabled: string[];
	    unknown: string[];
	
	    static createFrom(source: any = {}) {
	        return new SwitchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.job = source["job"];
	        this.updated = source["updated"];
	        this.disabled = source["disabled"];
	        this.unknown = source["unknown"];
	    }
	}
	export class TabInfo {
	    shaderMod: string;
	    worlds: string[];
	
	    static createFrom(source: any = {}) {
	        return new TabInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.shaderMod = source["shaderMod"];
	        this.worlds = source["worlds"];
	    }
	}

}

export namespace docs {
	
	export class Span {
	    text: string;
	    code?: boolean;
	    href?: string;
	
	    static createFrom(source: any = {}) {
	        return new Span(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.code = source["code"];
	        this.href = source["href"];
	    }
	}
	export class Block {
	    type: string;
	    spans?: Span[];
	    items?: Span[][];
	    src?: string;
	    alt?: string;
	
	    static createFrom(source: any = {}) {
	        return new Block(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.spans = this.convertValues(source["spans"], Span);
	        this.items = this.convertValues(source["items"], Span);
	        this.src = source["src"];
	        this.alt = source["alt"];
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
	export class EnvVar {
	    name: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvVar(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class CommandSettings {
	    preLaunch: string;
	    wrapper: string;
	    postExit: string;
	    env: EnvVar[];
	    systemGlfw: boolean;
	    systemOpenal: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CommandSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preLaunch = source["preLaunch"];
	        this.wrapper = source["wrapper"];
	        this.postExit = source["postExit"];
	        this.env = this.convertValues(source["env"], EnvVar);
	        this.systemGlfw = source["systemGlfw"];
	        this.systemOpenal = source["systemOpenal"];
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
	export class CreateRequest {
	    name: string;
	    art: Art;
	    icon: string;
	    minecraft: string;
	    loader: string;
	    loaderVersion: string;
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
	        this.icon = source["icon"];
	        this.minecraft = source["minecraft"];
	        this.loader = source["loader"];
	        this.loaderVersion = source["loaderVersion"];
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
	
	export class Group {
	    name: string;
	    collapsed?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.collapsed = source["collapsed"];
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
	export class PerformanceSettings {
	    dedicatedGpu: boolean;
	    gameMode: boolean;
	    mangoHud: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PerformanceSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dedicatedGpu = source["dedicatedGpu"];
	        this.gameMode = source["gameMode"];
	        this.mangoHud = source["mangoHud"];
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
	    commands: CommandSettings;
	    performance: PerformanceSettings;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.java = this.convertValues(source["java"], JavaSettings);
	        this.window = this.convertValues(source["window"], WindowSettings);
	        this.commands = this.convertValues(source["commands"], CommandSettings);
	        this.performance = this.convertValues(source["performance"], PerformanceSettings);
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
	export class TrashEntry {
	    key: string;
	    id: string;
	    name: string;
	    art: Art;
	    icon?: string;
	    minecraft: string;
	    loader?: string;
	    group?: string;
	    trashedAt: number;
	    location?: string;
	
	    static createFrom(source: any = {}) {
	        return new TrashEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.art = this.convertValues(source["art"], Art);
	        this.icon = source["icon"];
	        this.minecraft = source["minecraft"];
	        this.loader = source["loader"];
	        this.group = source["group"];
	        this.trashedAt = source["trashedAt"];
	        this.location = source["location"];
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
	    mods: number;
	
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
	        this.mods = source["mods"];
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

export namespace manual {
	
	export class Item {
	    id: string;
	    name: string;
	    fileName: string;
	    size: number;
	    state: string;
	    twin: boolean;
	    page?: string;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.fileName = source["fileName"];
	        this.size = source["size"];
	        this.state = source["state"];
	        this.twin = source["twin"];
	        this.page = source["page"];
	    }
	}
	export class View {
	    jobId: string;
	    folder: string;
	    items: Item[];
	    done: boolean;
	
	    static createFrom(source: any = {}) {
	        return new View(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobId = source["jobId"];
	        this.folder = source["folder"];
	        this.items = this.convertValues(source["items"], Item);
	        this.done = source["done"];
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

export namespace modpack {
	
	export class Available {
	    instanceId: string;
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new Available(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.instanceId = source["instanceId"];
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}
	export class DiffRow {
	    name: string;
	    version: string;
	    kind: string;
	
	    static createFrom(source: any = {}) {
	        return new DiffRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.kind = source["kind"];
	    }
	}
	export class Diff {
	    from: string;
	    to: string;
	    summary: string;
	    keptEdits: string[];
	    rows: DiffRow[];
	    game?: string;
	
	    static createFrom(source: any = {}) {
	        return new Diff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	        this.summary = source["summary"];
	        this.keptEdits = source["keptEdits"];
	        this.rows = this.convertValues(source["rows"], DiffRow);
	        this.game = source["game"];
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
	
	export class Stats {
	    mods: number;
	    modBytes: number;
	    files: number;
	    folders: string[];
	    blocked: number;
	    foreign: string[];
	    needsWorld?: number;
	
	    static createFrom(source: any = {}) {
	        return new Stats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mods = source["mods"];
	        this.modBytes = source["modBytes"];
	        this.files = source["files"];
	        this.folders = source["folders"];
	        this.blocked = source["blocked"];
	        this.foreign = source["foreign"];
	        this.needsWorld = source["needsWorld"];
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

export namespace store {
	
	export class Usage {
	    blobs: number;
	    bytes: number;
	    unused: number;
	    unusedBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new Usage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blobs = source["blobs"];
	        this.bytes = source["bytes"];
	        this.unused = source["unused"];
	        this.unusedBytes = source["unusedBytes"];
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

