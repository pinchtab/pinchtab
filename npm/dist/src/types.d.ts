export interface SnapshotParams {
    refs?: 'role' | 'aria';
    selector?: string;
    maxTokens?: number;
    format?: 'full' | 'compact';
}
export interface SnapshotResponse {
    html: string;
    refs?: Record<string, any>;
}
export interface TabClickParams {
    ref: string;
    targetId?: string;
}
export interface TabLockParams {
    tabId: string;
    timeoutMs?: number;
}
export interface TabUnlockParams {
    tabId: string;
}
export interface CreateTabParams {
    url: string;
    stealth?: 'light' | 'full';
}
export interface CreateTabResponse {
    tabId: string;
}
export interface PinchtabOptions {
    baseUrl?: string;
    timeout?: number;
    port?: number;
}
