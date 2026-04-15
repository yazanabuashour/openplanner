package app

const (
	name    = "openplanner"
	tagline = "Planning tooling is under construction."
)

// Banner returns the initial CLI banner for the bootstrap binary.
func Banner() string {
	return name + ": " + tagline
}
