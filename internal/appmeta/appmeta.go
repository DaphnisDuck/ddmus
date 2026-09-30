package appmeta

import "github.com/bjarneo/cliamp/internal/appdir"

var (
	clientName = appdir.Name // ddmus: identifies this fork to servers and MPRIS
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
// (the UI header, MPRIS Identity). ddmus.
func DisplayName() string { return "DaphnisDuck's Music Player" }

func DeviceName() string { return deviceName }

func Version() string { return version }
