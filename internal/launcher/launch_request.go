package launcher

// LaunchRequest is captured on submission. Browsing another instance or account
// does not alter this operation. Use Snapshot before handing it to a worker.
type LaunchRequest struct {
	Instance       Instance
	State          State
	Account        MicrosoftAccount
	Session        MinecraftSession
	StationSession StationSession
	Server         string
	InstallOnly    bool
}

func (r LaunchRequest) Snapshot() LaunchRequest {
	r.State = r.State.Clone()
	return r
}

// LaunchResult carries the latest credentials and committed state even on error.
// CredentialsChanged means Microsoft credentials or the session changed in memory;
// CredentialsSaved reports encrypted persistence for the selected provider.
// Failed authentication or saving prevents launch; retained sessions must pass
// authorization and persistence again on retry. Running is not game readiness.
type LaunchResult struct {
	Running            Running
	Account            MicrosoftAccount
	Session            MinecraftSession
	StationSession     StationSession
	State              State
	CredentialsChanged bool
	CredentialsSaved   bool
	StateChanged       bool
	AuthFailed         bool
}
