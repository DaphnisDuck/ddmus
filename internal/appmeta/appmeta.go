package appmeta

import (
	"strings"

	"github.com/bjarneo/cliamp/internal/appdir"
)

var (
	clientName = appdir.Name // ddsonic: identifies this fork to servers and MPRIS
	deviceName = appdir.Name
	version    = "dev"
)

func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func ClientName() string { return clientName }

// DisplayName is the player's full name, for people rather than servers
// (the UI header, MPRIS Identity). ddsonic.
func DisplayName() string { return "DaphnisDuck's Music Player" }

func DeviceName() string { return deviceName }

func Version() string { return version }

// CallbackPage is one of upstream's sign-in callback pages, titled with this
// player's name instead of cliamp's: the browser tab a user sees after
// signing in to Spotify or YouTube Music. ddsonic.
func CallbackPage(html string) string {
	return strings.Replace(html, "<title>cliamp</title>", "<title>"+clientName+"</title>", 1)
}

// projectURL is where a server's operator can find out what this client is.
const projectURL = "https://github.com/DaphnisDuck/ddsonic"

// UserAgent is how ddsonic names itself in the HTTP requests it makes, as
// "ddsonic/<version> (<project URL>)". It is the one place that string is
// built: every request that names its client calls this. ddsonic.
func UserAgent() string {
	return clientName + "/" + strings.TrimPrefix(version, "v") + " (" + projectURL + ")"
}
