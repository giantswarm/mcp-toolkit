package brand

import (
	_ "embed"
	"encoding/base64"
)

//go:embed giantswarm.svg
var logo []byte

const MIMEType = "image/svg+xml"

func DataURI(svg []byte) string {
	return "data:" + MIMEType + ";base64," + base64.StdEncoding.EncodeToString(svg)
}

func GiantSwarmDataURI() string { return DataURI(logo) }
