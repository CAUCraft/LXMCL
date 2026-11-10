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

// LaunchResult carries committed credential/state changes even when a later
// install or process start fails. Running means Java started, not game readiness.
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
