import { SnapshotParams, SnapshotResponse, TabClickParams, TabLockParams, TabUnlockParams, CreateTabParams, CreateTabResponse, PinchtabOptions } from './types';
export * from './types';
export declare class Pinchtab {
    private baseUrl;
    private timeout;
    private port;
    private process;
    private binaryPath;
    constructor(options?: PinchtabOptions);
    /**
     * Start the Pinchtab server process
     */
    start(binaryPath?: string): Promise<void>;
    /**
     * Stop the Pinchtab server process
     */
    stop(): Promise<void>;
    /**
     * Take a snapshot of the current tab
     */
    snapshot(params?: SnapshotParams): Promise<SnapshotResponse>;
    /**
     * Click on a UI element
     */
    click(params: TabClickParams): Promise<void>;
    /**
     * Lock a tab
     */
    lock(params: TabLockParams): Promise<void>;
    /**
     * Unlock a tab
     */
    unlock(params: TabUnlockParams): Promise<void>;
    /**
     * Create a new tab
     */
    createTab(params: CreateTabParams): Promise<CreateTabResponse>;
    /**
     * Make a request to the Pinchtab API
     */
    private request;
    /**
     * Get the path to the Pinchtab binary
     */
    private getBinaryPath;
    private getBinaryName;
}
export default Pinchtab;
