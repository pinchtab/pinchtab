package scheduler

import "time"

// timeNow is the clock function used by the scheduler package.
// Tests can override this for deterministic behavior.
var timeNow = time.Now
