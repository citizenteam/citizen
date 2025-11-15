package k3s

import _ "embed"

//go:embed traefik-crds.yaml
var embeddedTraefikCRDs string
