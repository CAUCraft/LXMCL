package launcher

import (
	"net/http"

	"github.com/CAUCraft/LXMCL/internal/testutil/mcfixture"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

var newInstallFixture = mcfixture.New
var fixtureResponse = mcfixture.Response
