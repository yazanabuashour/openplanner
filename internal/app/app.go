package app

const (
	name    = "openplanner"
	tagline = "Spec-first local planning SDK."
)

// Banner returns the initial CLI banner for the bootstrap binary.
func Banner() string {
	return name + ": " + tagline
}
