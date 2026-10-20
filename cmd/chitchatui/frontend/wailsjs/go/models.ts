export namespace crushfling {
	
	export class AccountResult {
	    name?: string;
	    username: string;
	    email: string;
	    password?: string;
	    age: number;
	    gender: string;
	    looking_for: string;
	    country: string;
	    city?: string;
	    platform: string;
	    platform_username: string;
	    bio: string;
	    avatar_path?: string;
	    avatar_url?: string;
	    tags?: string[];
	    // Go type: time
	    created_at: any;
	    magic_link: string;
	    session_cookies: Record<string, string>;
	    local_storage?: Record<string, string>;
	    user_id?: string;
	    verified: boolean;
	    error?: string;
	    proxy_used?: string;
	    raw_response?: string;
	    error_code?: string;
	    failed_step?: string;
	    duration_ms?: number;
	    timings?: Record<string, number>;
	    profile_completed: boolean;
	    profile_verified?: boolean;
	    profile_error?: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.username = source["username"];
	        this.email = source["email"];
	        this.password = source["password"];
	        this.age = source["age"];
	        this.gender = source["gender"];
	        this.looking_for = source["looking_for"];
	        this.country = source["country"];
	        this.city = source["city"];
	        this.platform = source["platform"];
	        this.platform_username = source["platform_username"];
	        this.bio = source["bio"];
	        this.avatar_path = source["avatar_path"];
	        this.avatar_url = source["avatar_url"];
	        this.tags = source["tags"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.magic_link = source["magic_link"];
	        this.session_cookies = source["session_cookies"];
	        this.local_storage = source["local_storage"];
	        this.user_id = source["user_id"];
	        this.verified = source["verified"];
	        this.error = source["error"];
	        this.proxy_used = source["proxy_used"];
	        this.raw_response = source["raw_response"];
	        this.error_code = source["error_code"];
	        this.failed_step = source["failed_step"];
	        this.duration_ms = source["duration_ms"];
	        this.timings = source["timings"];
	        this.profile_completed = source["profile_completed"];
	        this.profile_verified = source["profile_verified"];
	        this.profile_error = source["profile_error"];
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
	export class BioInfo {
	    path: string;
	    file_name: string;
	    remaining: number;
	    total: number;
	    preview: string[];
	
	    static createFrom(source: any = {}) {
	        return new BioInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.file_name = source["file_name"];
	        this.remaining = source["remaining"];
	        this.total = source["total"];
	        this.preview = source["preview"];
	    }
	}
	export class RegistrationStats {
	    total: number;
	    success: number;
	    failed: number;
	    pending: number;
	    running: boolean;
	    last_error?: string;
	    by_code?: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new RegistrationStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.success = source["success"];
	        this.failed = source["failed"];
	        this.pending = source["pending"];
	        this.running = source["running"];
	        this.last_error = source["last_error"];
	        this.by_code = source["by_code"];
	    }
	}
	export class DebugInfo {
	    data_dir: string;
	    log_path: string;
	    trace_path: string;
	    debug: boolean;
	    trace: boolean;
	    stats: RegistrationStats;
	
	    static createFrom(source: any = {}) {
	        return new DebugInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data_dir = source["data_dir"];
	        this.log_path = source["log_path"];
	        this.trace_path = source["trace_path"];
	        this.debug = source["debug"];
	        this.trace = source["trace"];
	        this.stats = this.convertValues(source["stats"], RegistrationStats);
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
	export class ProfileData {
	    name?: string;
	    username: string;
	    email: string;
	    password?: string;
	    age: number;
	    gender: string;
	    looking_for: string;
	    country: string;
	    city?: string;
	    platform: string;
	    platform_username: string;
	    bio: string;
	    avatar_path?: string;
	    avatar_url?: string;
	    tags?: string[];
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ProfileData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.username = source["username"];
	        this.email = source["email"];
	        this.password = source["password"];
	        this.age = source["age"];
	        this.gender = source["gender"];
	        this.looking_for = source["looking_for"];
	        this.country = source["country"];
	        this.city = source["city"];
	        this.platform = source["platform"];
	        this.platform_username = source["platform_username"];
	        this.bio = source["bio"];
	        this.avatar_path = source["avatar_path"];
	        this.avatar_url = source["avatar_url"];
	        this.tags = source["tags"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class ProxyCheck {
	    total: number;
	    valid: number;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProxyCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.valid = source["valid"];
	        this.errors = source["errors"];
	    }
	}
	export class RegisterConfig {
	    data_dir: string;
	    count: number;
	    concurrency: number;
	    headless: boolean;
	    use_browser: boolean;
	    proxy_file: string;
	    proxy_entries: string;
	    email_provider: string;
	    custom_email: string;
	    custom_email_hook: string;
	    max_retries: number;
	    mail_poll_sec: number;
	    mail_timeout_sec: number;
	    avatar_dir: string;
	    min_delay_ms: number;
	    max_delay_ms: number;
	    disable_cf_bypass: boolean;
	    captcha_api_key: string;
	    output_file: string;
	    manual_link: boolean;
	    pasted_link: string;
	    browser_verify: boolean;
	    debug: boolean;
	    trace_http: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RegisterConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.data_dir = source["data_dir"];
	        this.count = source["count"];
	        this.concurrency = source["concurrency"];
	        this.headless = source["headless"];
	        this.use_browser = source["use_browser"];
	        this.proxy_file = source["proxy_file"];
	        this.proxy_entries = source["proxy_entries"];
	        this.email_provider = source["email_provider"];
	        this.custom_email = source["custom_email"];
	        this.custom_email_hook = source["custom_email_hook"];
	        this.max_retries = source["max_retries"];
	        this.mail_poll_sec = source["mail_poll_sec"];
	        this.mail_timeout_sec = source["mail_timeout_sec"];
	        this.avatar_dir = source["avatar_dir"];
	        this.min_delay_ms = source["min_delay_ms"];
	        this.max_delay_ms = source["max_delay_ms"];
	        this.disable_cf_bypass = source["disable_cf_bypass"];
	        this.captcha_api_key = source["captcha_api_key"];
	        this.output_file = source["output_file"];
	        this.manual_link = source["manual_link"];
	        this.pasted_link = source["pasted_link"];
	        this.browser_verify = source["browser_verify"];
	        this.debug = source["debug"];
	        this.trace_http = source["trace_http"];
	    }
	}

}

export namespace main {
	
	export class BotLogEntry {
	    seq: number;
	    time: string;
	    kind: string;
	    text?: string;
	    rule?: string;
	    detail?: string;
	    level?: string;
	    profile?: string;
	    setting_ok: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BotLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.time = source["time"];
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.rule = source["rule"];
	        this.detail = source["detail"];
	        this.level = source["level"];
	        this.profile = source["profile"];
	        this.setting_ok = source["setting_ok"];
	    }
	}
	export class BotLogSnapshot {
	    version: number;
	    entries: BotLogEntry[];
	    dropped: number;
	
	    static createFrom(source: any = {}) {
	        return new BotLogSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.entries = this.convertValues(source["entries"], BotLogEntry);
	        this.dropped = source["dropped"];
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
	export class CrushSummary {
	    running: boolean;
	    pictures: number;
	    bios: number;
	    accounts: number;
	    verified: number;
	    browsers: number;
	    bio_file: string;
	    picture_dir: string;
	    data_dir: string;
	    accounts_csv: string;
	    stats: crushfling.RegistrationStats;
	
	    static createFrom(source: any = {}) {
	        return new CrushSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.pictures = source["pictures"];
	        this.bios = source["bios"];
	        this.accounts = source["accounts"];
	        this.verified = source["verified"];
	        this.browsers = source["browsers"];
	        this.bio_file = source["bio_file"];
	        this.picture_dir = source["picture_dir"];
	        this.data_dir = source["data_dir"];
	        this.accounts_csv = source["accounts_csv"];
	        this.stats = this.convertValues(source["stats"], crushfling.RegistrationStats);
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
	export class ProfileOption {
	    name: string;
	    has_session: boolean;
	    saved_at?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.has_session = source["has_session"];
	        this.saved_at = source["saved_at"];
	    }
	}
	export class ShowLogEntry {
	    seq: number;
	    time: string;
	    kind: string;
	    text?: string;
	    rule?: string;
	    detail?: string;
	    level?: string;
	    setting_ok: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ShowLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.time = source["time"];
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.rule = source["rule"];
	        this.detail = source["detail"];
	        this.level = source["level"];
	        this.setting_ok = source["setting_ok"];
	    }
	}
	export class ShowLogSnapshot {
	    version: number;
	    entries: ShowLogEntry[];
	    dropped: number;
	
	    static createFrom(source: any = {}) {
	        return new ShowLogSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.entries = this.convertValues(source["entries"], ShowLogEntry);
	        this.dropped = source["dropped"];
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
	export class UnifiedSettingsPayload {
	    first_message: model.FirstMessageSettings;
	    reply_delay_min_seconds: number;
	    reply_delay_max_seconds: number;
	    auto_skip: boolean;
	    no_reply_min_seconds: number;
	    no_reply_max_seconds: number;
	    next_chat_min_seconds: number;
	    next_chat_max_seconds: number;
	    poll_min_seconds: number;
	    poll_max_seconds: number;
	    action_min_seconds: number;
	    action_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new UnifiedSettingsPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.first_message = this.convertValues(source["first_message"], model.FirstMessageSettings);
	        this.reply_delay_min_seconds = source["reply_delay_min_seconds"];
	        this.reply_delay_max_seconds = source["reply_delay_max_seconds"];
	        this.auto_skip = source["auto_skip"];
	        this.no_reply_min_seconds = source["no_reply_min_seconds"];
	        this.no_reply_max_seconds = source["no_reply_max_seconds"];
	        this.next_chat_min_seconds = source["next_chat_min_seconds"];
	        this.next_chat_max_seconds = source["next_chat_max_seconds"];
	        this.poll_min_seconds = source["poll_min_seconds"];
	        this.poll_max_seconds = source["poll_max_seconds"];
	        this.action_min_seconds = source["action_min_seconds"];
	        this.action_max_seconds = source["action_max_seconds"];
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
	export class homeChatMessage {
	    id: string;
	    role: string;
	    text: string;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new homeChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.order = source["order"];
	    }
	}
	export class homeChatSessionInfo {
	    id: string;
	    label: string;
	    active: boolean;
	    dropped: number;
	
	    static createFrom(source: any = {}) {
	        return new homeChatSessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.active = source["active"];
	        this.dropped = source["dropped"];
	    }
	}
	export class homeChatSnapshot {
	    version: number;
	    selected: string;
	    sessions: homeChatSessionInfo[];
	    messages: homeChatMessage[];
	    evicted: number;
	
	    static createFrom(source: any = {}) {
	        return new homeChatSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.selected = source["selected"];
	        this.sessions = this.convertValues(source["sessions"], homeChatSessionInfo);
	        this.messages = this.convertValues(source["messages"], homeChatMessage);
	        this.evicted = source["evicted"];
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

export namespace model {
	
	export class BrowserOption {
	    key: string;
	    label: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new BrowserOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.description = source["description"];
	    }
	}
	export class BrowserSettings {
	    profile: string;
	    backend: string;
	    headless: boolean;
	    use_saved_session: boolean;
	    live_chat?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BrowserSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = source["profile"];
	        this.backend = source["backend"];
	        this.headless = source["headless"];
	        this.use_saved_session = source["use_saved_session"];
	        this.live_chat = source["live_chat"];
	    }
	}
	export class FirstMessageSettings {
	    mode: string;
	    bot_first_percent: number;
	    wait_timeout: boolean;
	    wait_min_seconds: number;
	    wait_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new FirstMessageSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.bot_first_percent = source["bot_first_percent"];
	        this.wait_timeout = source["wait_timeout"];
	        this.wait_min_seconds = source["wait_min_seconds"];
	        this.wait_max_seconds = source["wait_max_seconds"];
	    }
	}
	export class GeneralSettings {
	    first_message: FirstMessageSettings;
	    poll_min_seconds: number;
	    poll_max_seconds: number;
	    action_min_seconds: number;
	    action_max_seconds: number;
	    match_reply_timeout_min_seconds: number;
	    match_reply_timeout_max_seconds: number;
	    match_close_wait_min_seconds: number;
	    match_close_wait_max_seconds: number;
	    reply_delay_min_seconds: number;
	    reply_delay_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new GeneralSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.first_message = this.convertValues(source["first_message"], FirstMessageSettings);
	        this.poll_min_seconds = source["poll_min_seconds"];
	        this.poll_max_seconds = source["poll_max_seconds"];
	        this.action_min_seconds = source["action_min_seconds"];
	        this.action_max_seconds = source["action_max_seconds"];
	        this.match_reply_timeout_min_seconds = source["match_reply_timeout_min_seconds"];
	        this.match_reply_timeout_max_seconds = source["match_reply_timeout_max_seconds"];
	        this.match_close_wait_min_seconds = source["match_close_wait_min_seconds"];
	        this.match_close_wait_max_seconds = source["match_close_wait_max_seconds"];
	        this.reply_delay_min_seconds = source["reply_delay_min_seconds"];
	        this.reply_delay_max_seconds = source["reply_delay_max_seconds"];
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
	export class HomeStats {
	    bot_accounts: number;
	    running_accounts: number;
	    user_send_count: number;
	    dead_accounts: number;
	    traffic_users: number;
	    messages_sent: number;
	    snap_tg_sent_count: number;
	    skipped_accounts: number;
	
	    static createFrom(source: any = {}) {
	        return new HomeStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bot_accounts = source["bot_accounts"];
	        this.running_accounts = source["running_accounts"];
	        this.user_send_count = source["user_send_count"];
	        this.dead_accounts = source["dead_accounts"];
	        this.traffic_users = source["traffic_users"];
	        this.messages_sent = source["messages_sent"];
	        this.snap_tg_sent_count = source["snap_tg_sent_count"];
	        this.skipped_accounts = source["skipped_accounts"];
	    }
	}
	export class LogEntry {
	    time: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.message = source["message"];
	    }
	}
	export class MessageSettings {
	    chat_mode: string;
	    sequence: string;
	    usernames: string;
	    first_message?: string;
	    reply_message?: string;
	    loaded_sequences?: string[];
	    use_loaded_sequences: boolean;
	    keyword_rules_content?: string;
	    keyword_file_path?: string;
	    ai_enabled?: boolean;
	    ai_base_url?: string;
	    ai_api_key?: string;
	    ai_model?: string;
	
	    static createFrom(source: any = {}) {
	        return new MessageSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chat_mode = source["chat_mode"];
	        this.sequence = source["sequence"];
	        this.usernames = source["usernames"];
	        this.first_message = source["first_message"];
	        this.reply_message = source["reply_message"];
	        this.loaded_sequences = source["loaded_sequences"];
	        this.use_loaded_sequences = source["use_loaded_sequences"];
	        this.keyword_rules_content = source["keyword_rules_content"];
	        this.keyword_file_path = source["keyword_file_path"];
	        this.ai_enabled = source["ai_enabled"];
	        this.ai_base_url = source["ai_base_url"];
	        this.ai_api_key = source["ai_api_key"];
	        this.ai_model = source["ai_model"];
	    }
	}
	export class ProxySettings {
	    enabled: boolean;
	    entries: string;
	    address?: string;
	    username?: string;
	    password?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.entries = source["entries"];
	        this.address = source["address"];
	        this.username = source["username"];
	        this.password = source["password"];
	    }
	}
	export class ShowModeSettings {
	    first_message: FirstMessageSettings;
	    reply_delay_min_seconds: number;
	    reply_delay_max_seconds: number;
	    auto_skip: boolean;
	    no_reply_min_seconds: number;
	    no_reply_max_seconds: number;
	    next_chat_min_seconds: number;
	    next_chat_max_seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new ShowModeSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.first_message = this.convertValues(source["first_message"], FirstMessageSettings);
	        this.reply_delay_min_seconds = source["reply_delay_min_seconds"];
	        this.reply_delay_max_seconds = source["reply_delay_max_seconds"];
	        this.auto_skip = source["auto_skip"];
	        this.no_reply_min_seconds = source["no_reply_min_seconds"];
	        this.no_reply_max_seconds = source["no_reply_max_seconds"];
	        this.next_chat_min_seconds = source["next_chat_min_seconds"];
	        this.next_chat_max_seconds = source["next_chat_max_seconds"];
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
	export class UISnapshot {
	    running: boolean;
	    show_mode_running: boolean;
	    license_invalid: boolean;
	    license_key: string;
	    license_reason: string;
	    home: HomeStats;
	    browser: BrowserSettings;
	    message: MessageSettings;
	    proxy: ProxySettings;
	    general: GeneralSettings;
	    show_mode: ShowModeSettings;
	    browser_options: BrowserOption[];
	    logs: LogEntry[];
	
	    static createFrom(source: any = {}) {
	        return new UISnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.show_mode_running = source["show_mode_running"];
	        this.license_invalid = source["license_invalid"];
	        this.license_key = source["license_key"];
	        this.license_reason = source["license_reason"];
	        this.home = this.convertValues(source["home"], HomeStats);
	        this.browser = this.convertValues(source["browser"], BrowserSettings);
	        this.message = this.convertValues(source["message"], MessageSettings);
	        this.proxy = this.convertValues(source["proxy"], ProxySettings);
	        this.general = this.convertValues(source["general"], GeneralSettings);
	        this.show_mode = this.convertValues(source["show_mode"], ShowModeSettings);
	        this.browser_options = this.convertValues(source["browser_options"], BrowserOption);
	        this.logs = this.convertValues(source["logs"], LogEntry);
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

export namespace proxy {
	
	export class TestResult {
	    line: number;
	    label: string;
	    ok: boolean;
	    tunnel_ok: boolean;
	    status: number;
	    latency_ms: number;
	    detail: string;
	    hint?: string;
	    provider_message?: string;
	    proxy_authenticate?: string;
	    authenticated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.line = source["line"];
	        this.label = source["label"];
	        this.ok = source["ok"];
	        this.tunnel_ok = source["tunnel_ok"];
	        this.status = source["status"];
	        this.latency_ms = source["latency_ms"];
	        this.detail = source["detail"];
	        this.hint = source["hint"];
	        this.provider_message = source["provider_message"];
	        this.proxy_authenticate = source["proxy_authenticate"];
	        this.authenticated = source["authenticated"];
	    }
	}

}

