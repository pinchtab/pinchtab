package autosolver

import "time"

// IntentType classifies the detected page state.
type IntentType string

const (
	IntentNormal     IntentType = "normal"     // No challenge or blocker detected
	IntentCaptcha    IntentType = "captcha"    // CAPTCHA challenge (Turnstile, reCAPTCHA, hCaptcha)
	IntentLogin      IntentType = "login"      // Login form detected
	IntentSignup     IntentType = "signup"     // Signup/registration form detected
	IntentForm       IntentType = "form"       // Generic form-filling flow detected
	IntentBlocked    IntentType = "blocked"    // Navigation blocked (interstitial, bot gate)
	IntentOnboarding IntentType = "onboarding" // Multi-step onboarding flow
	IntentNavigation IntentType = "navigation" // Multi-step navigation task
	IntentUnknown    IntentType = "unknown"    // Page state unclear
)

// ActionType describes the kind of browser action to perform.
type ActionType string

const (
	ActionClick    ActionType = "click"
	ActionType_    ActionType = "type"
	ActionWait     ActionType = "wait"
	ActionNavigate ActionType = "navigate"
	ActionEvaluate ActionType = "evaluate"
	ActionNone     ActionType = "none" // No action needed
)

// SolverStatus indicates the outcome of a solver attempt.
type SolverStatus string

const (
	StatusSolved  SolverStatus = "solved"
	StatusFailed  SolverStatus = "failed"
	StatusSkipped SolverStatus = "skipped"
	StatusTimeout SolverStatus = "timeout"
)

// Result is the outcome of an autosolver run.
type Result struct {
	Solved        bool           `json:"solved"`
	SolverUsed    string         `json:"solverUsed,omitempty"`
	Intent        IntentType     `json:"intent,omitempty"`
	Attempts      int            `json:"attempts"`
	TotalDuration time.Duration  `json:"totalDuration"`
	History       []AttemptEntry `json:"history,omitempty"`
	FinalTitle    string         `json:"finalTitle,omitempty"`
	FinalURL      string         `json:"finalURL,omitempty"`
	Error         string         `json:"error,omitempty"`
}

// AttemptEntry records a single solver attempt within the fallback chain.
type AttemptEntry struct {
	Solver   string        `json:"solver"`
	Status   SolverStatus  `json:"status"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// Intent represents the autosolver's understanding of the current page.
type Intent struct {
	Type       IntentType `json:"type"`
	Confidence float64    `json:"confidence"` // 0.0 to 1.0
	Details    string     `json:"details,omitempty"`

	// ChallengeType provides extra detail for captcha/blocked intents
	// (e.g., "turnstile", "recaptcha-v2", "hcaptcha", "interstitial").
	ChallengeType string `json:"challengeType,omitempty"`
}

// ElementMatch represents a matched UI element on the page.
type ElementMatch struct {
	Ref        string  `json:"ref,omitempty"`
	Role       string  `json:"role,omitempty"`
	Name       string  `json:"name,omitempty"`
	Selector   string  `json:"selector,omitempty"`
	Score      float64 `json:"score"`
	Confidence string  `json:"confidence,omitempty"` // "high", "medium", "low"
	X          float64 `json:"x,omitempty"`
	Y          float64 `json:"y,omitempty"`
}

// SuggestedAction is the autosolver's recommended next step.
type SuggestedAction struct {
	Action   ActionType `json:"action"`
	Selector string     `json:"selector,omitempty"`
	Text     string     `json:"text,omitempty"`
	URL      string     `json:"url,omitempty"`
	Expr     string     `json:"expr,omitempty"`
	X        float64    `json:"x,omitempty"`
	Y        float64    `json:"y,omitempty"`
	Reason   string     `json:"reason,omitempty"` // Why this action was chosen
}

// LLMRequest is the input to the LLM fallback provider.
// Fields are designed to minimize token usage.
type LLMRequest struct {
	PageTitle    string         `json:"pageTitle"`
	PageURL      string         `json:"pageUrl"`
	TrimmedHTML  string         `json:"trimmedHtml"`  // Stripped of scripts/styles
	DetectedType IntentType     `json:"detectedType"` // What we think the page is
	PrevAttempts []AttemptEntry `json:"prevAttempts"` // Failed attempts so far
}

// LLMResponse is the LLM's recommended action.
type LLMResponse struct {
	Action     ActionType `json:"action"`
	Selector   string     `json:"selector,omitempty"`
	Text       string     `json:"text,omitempty"`
	URL        string     `json:"url,omitempty"`
	Reasoning  string     `json:"reasoning,omitempty"`
	Confidence float64    `json:"confidence"`
}

// Config holds autosolver runtime configuration.
type Config struct {
	Enabled        bool          `json:"enabled"`
	MaxAttempts    int           `json:"maxAttempts"`
	SolverTimeout  time.Duration `json:"solverTimeout"`
	Solvers        []string      `json:"solvers"`        // Ordered solver names to try
	LLMFallback    bool          `json:"llmFallback"`    // Enable LLM as last resort
	RetryBaseDelay time.Duration `json:"retryBaseDelay"` // Base delay for exponential backoff
	RetryMaxDelay  time.Duration `json:"retryMaxDelay"`  // Cap for exponential backoff
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:        true,
		MaxAttempts:    8,
		SolverTimeout:  30 * time.Second,
		Solvers:        []string{"cloudflare", "semantic"},
		LLMFallback:    false,
		RetryBaseDelay: 500 * time.Millisecond,
		RetryMaxDelay:  10 * time.Second,
	}
}
