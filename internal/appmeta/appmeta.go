package appmeta

import "github.com/bjarneo/cliamp/internal/appdir"

var (
	clientName = appdir.Name // omatunes: identifies this fork to servers and MPRIS
	deviceName = appdir.Name
	version    = "dev"
)

func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func ClientName() string { return clientName }

func DeviceName() string { return deviceName }

func Version() string { return version }
