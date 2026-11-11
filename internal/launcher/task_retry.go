package launcher

import "github.com/CAUCraft/LXMCL/internal/tasks"

func RetryTarget(r tasks.Record, s State) (Instance, bool) {
	if r.InstanceID == 0 || (r.Status != tasks.Failed && r.Status != tasks.Canceled) ||
		(r.Action != tasks.Launch && r.Action != tasks.Verify && r.Resource == nil) {
		return Instance{}, false
	}
	if r.Action == tasks.Launch && (r.Account != "" && r.Account != s.AccountKey() ||
		r.Account == "" && (s.MicrosoftProfile != "" || s.StationKey != "")) {
		return Instance{}, false
	}
	i, ok := s.Find(r.InstanceID)
	return i, ok && i.Revision == r.Revision && i.Version == r.Version &&
		(r.Resource == nil || (r.Resource.Project != "" && r.Resource.Version != "" && ContentKind(r.Resource.Kind).Supports(i)))
}
