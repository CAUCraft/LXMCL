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

// LaunchResult carries refreshed credentials and committed state changes even
// when authentication, installation or startup fails. CredentialsChanged means
// Microsoft credentials were refreshed in memory, not necessarily saved; a save
// failure is returned with an empty Session. StateChanged means State was saved.
// Running means Java started, not game readiness.
type LaunchResult struct {
	Running            Running
	Account            MicrosoftAccount
	Session            MinecraftSession
	StationSession     StationSession
	State              State
	CredentialsChanged bool
	StateChanged       bool
	AuthFailed         bool
}
