package provider

// DnetProvider is entity that runs on top of dnet. For eg cni
// TODO: This interface may be removed/expanded depending on the push/pull
// model we might adopt.
type DnetProvider interface {
	// FetchActiveSandboxes fetches the active sandboxes as map
	// of sandbox IDs and sandbox options
	FetchActiveSandboxes() (map[string]interface{}, error)
}
