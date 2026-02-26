package bridge

// AdBlockPatterns contains patterns for common ad, tracking, and analytics domains
// to provide cleaner snapshots for AI agents
var AdBlockPatterns = []string{
	// Google Analytics & Tag Manager
	"*google-analytics.com/*",
	"*googletagmanager.com/*",
	"*googletagservices.com/*",
	"*googlesyndication.com/*",
	"*googleadservices.com/*",
	"*doubleclick.net/*",

	// Facebook/Meta tracking
	"*facebook.com/tr/*",
	"*facebook.com/plugins/*",
	"*connect.facebook.net/*",
	"*fbcdn.net/*/fbevents.js",

	// Twitter/X tracking
	"*twitter.com/i/adsct",
	"*analytics.twitter.com/*",
	"*static.ads-twitter.com/*",

	// Amazon tracking
	"*amazon-adsystem.com/*",
	"*amazontrust.com/*",
	"*adsafeprotected.com/*",

	// Common analytics services
	"*segment.io/*",
	"*segment.com/*",
	"*mixpanel.com/*",
	"*amplitude.com/*",
	"*mxpnl.com/*",
	"*kissmetrics.com/*",
	"*hotjar.com/*",
	"*fullstory.com/*",
	"*heapanalytics.com/*",
	"*mouseflow.com/*",
	"*luckyorange.com/*",
	"*crazyegg.com/*",
	"*pingdom.net/*",
	"*newrelic.com/*",
	"*nr-data.net/*",

	// Ad networks
	"*doubleclick.net/*",
	"*googleadservices.com/*",
	"*googlesyndication.com/*",
	"*adnxs.com/*",
	"*adsymptotic.com/*",
	"*openx.net/*",
	"*pubmatic.com/*",
	"*rubiconproject.com/*",
	"*adsrvr.org/*",
	"*amazon-adsystem.com/*",
	"*media.net/*",
	"*adtech.de/*",
	"*adzerk.net/*",
	"*criteo.com/*",
	"*criteo.net/*",
	"*casalemedia.com/*",
	"*33across.com/*",
	"*taboola.com/*",
	"*outbrain.com/*",
	"*revcontent.com/*",
	"*zemanta.com/*",
	"*disqus.com/ads/*",

	// Marketing automation
	"*marketo.com/*",
	"*marketo.net/*",
	"*hubspot.com/analytics/*",
	"*pardot.com/*",
	"*leadfeeder.com/*",
	"*clickcease.com/*",
	"*leadforensics.com/*",

	// Social media widgets (often slow/tracking)
	"*platform.twitter.com/widgets/*",
	"*platform.instagram.com/widgets/*",
	"*platform.linkedin.com/widgets/*",
	"*pinterest.com/js/pinit.js",

	// Common trackers
	"*scorecardresearch.com/*",
	"*quantserve.com/*",
	"*quantcount.com/*",
	"*parsely.com/*",
	"*chartbeat.com/*",
	"*omtrdc.net/*",
	"*optimizely.com/*",
	"*visualwebsiteoptimizer.com/*",
	"*demdex.net/*",
	"*bluekai.com/*",
	"*addthis.com/*",
	"*sharethis.com/*",

	// Cookie consent (often blocks content)
	"*cookielaw.org/*",
	"*cookiebot.com/*",
	"*onetrust.com/*",
	"*trustarc.com/*",
	"*usercentrics.com/*",

	// Misc tracking pixels
	"*pixel.gif*",
	"*tracking.gif*",
	"*analytics.gif*",
	"*/tr?*",
	"*/pixel?*",
	"*/collect?*",
}

// CombineBlockPatterns merges multiple pattern lists
func CombineBlockPatterns(patterns ...[]string) []string {
	var result []string
	seen := make(map[string]bool)

	for _, list := range patterns {
		for _, pattern := range list {
			if !seen[pattern] {
				seen[pattern] = true
				result = append(result, pattern)
			}
		}
	}

	return result
}
