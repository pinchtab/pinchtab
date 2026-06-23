# Browser Profile Lock Recovery

Chromium-based browsers use a `SingletonLock` file in their user data directory to prevent multiple instances from sharing the same profile simultaneously. If PinchTab or the browser crashes, this lock file (and associated `SingletonSocket` and `SingletonCookie` files) can be left behind, preventing the next PinchTab startup from succeeding with the error:

> "The profile appears to be in use by another Chromium process"

This document explains how PinchTab identifies, validates, and recovers from these stale locks while ensuring multi-instance safety.

## Recovery Mechanism

The recovery process follows a multi-layered approach to distinguish between a truly active profile and a stale one left by a crashed instance.

### 1. Detection

During initialization in `internal/bridge/runtime/init.go` (with bridge wiring in `internal/bridge/bridge_lifecycle.go`), if browser startup fails, PinchTab checks the error message for signatures of a profile lock:
- `The profile appears to be in use by another Chromium process`
- `The profile appears to be in use by another Chrome process`
- `process_singleton_posix.cc` (indicating a failure in the ProcessSingleton logic)

### 2. Validation & PinchTab PID Lock

To safely clear a lock, PinchTab must be certain no *other* active PinchTab instance is currently using that profile.

- **`pinchtab.pid`**: When a bridge starts, it writes its own PID to `$PROFILE_DIR/pinchtab.pid`.
- **Ownership Check**: Before clearing any browser locks, PinchTab reads this file.
  - If the PID in the file is still running **and** is verified to be a `pinchtab` process (by inspecting its command-line arguments), it assumes another PinchTab instance is active and **does not** touch the locks.
  - This verification prevents issues with PID reuse where a dead PinchTab instance's PID is reassigned to a different process.
  - If the PID is not running or is not a PinchTab process, the previous instance is considered "dead," and the profile is eligible for recovery.

### 3. Headless Fallback

If a headless PinchTab instance cannot acquire a lock on the requested profile directory (because another PinchTab instance is genuinely using it), it automatically falls back to creating a unique temporary profile directory. This allows multiple headless bridges to run concurrently even if they all default to the same profile path, while still preserving isolation and safety.

### 4a. Stale Process Termination

Even if the previous PinchTab instance is dead, orphaned browser processes might still be holding the profile lock.

- **Process Listing**: PinchTab scans the system process list for any processes launched with the same `--user-data-dir`.
- **Aggressive Cleanup**: If the `pinchtab.pid` check confirms no active owner, PinchTab sends `SIGKILL` to any orphaned browser processes associated with that profile. This is necessary because Chromium's internal "singleton" logic can be extremely stubborn if it thinks another process is even partially alive.

### 4b. Lock File Removal

Once stale processes are terminated, PinchTab removes the following files from the profile directory:
- `SingletonLock`
- `SingletonSocket`
- `SingletonCookie`

### 5. Automatic Retry

After clearing the stale state, `InitBrowser` automatically retries the startup sequence once. This makes the recovery transparent to the user and the API caller (e.g., the first `/health` check will succeed after a brief internal recovery delay).

## Implementation Details

The logic is distributed across these components:

- **`internal/bridge/profile_lock.go`**: Core logic for detection, PID lock management (`AcquireProfileLock`), and stale file removal.
- **`internal/bridge/profile_lock_pid_*.go`**: Platform-specific implementations for PID probing and process killing (supports Unix-like systems and Windows).
- **`internal/bridge/runtime/init.go`**: Orchestrates the retry logic within `startBrowserWithRecovery`.
- **`internal/bridge/bridge_lifecycle.go`**: Calls `AcquireProfileLock` and `EnsureBrowser` during bridge startup.
- **`internal/server/bridge.go`**: Ensures clean shutdown via signal handling to prevent locks from being left behind in the first place.

## Multi-Instance Safety

By combining the browser-level `SingletonLock` with the application-level `pinchtab.pid`, PinchTab achieves:
1. **Safety**: It never kills a browser being used by a healthy PinchTab instance.
2. **Resilience**: It automatically "self-heals" after a crash or power failure.
3. **Transparency**: Users don't need to manually `rm -rf` profile directories to fix "in use" errors.
