package devicebridge

import (
	"net/http"
	"testing"
	"time"
)

// F4 (2026-10-10 live rig): a device paired with v0.3.0 capabilities could not
// sign its connect request, so it never came online. Any valid canonical
// declaration must sign; malformed declarations must still be refused.
func TestConnectSigningAcceptsEveryNegotiatedDeclaration(t *testing.T) {
	var nonce [NonceBytes]byte
	now := time.Now()
	for _, caps := range [][]string{
		{ClassMachineStateQuery},
		{ClassNotifyLocal},
		{ClassMachineStateQuery, ClassNotifyLocal},
		{ClassShellExec},
		{ClassSessionQuery},
		{ClassMachineStateQuery, ClassNotifyLocal, ClassShellExec},
		{ClassMachineStateQuery, ClassNotifyLocal, ClassShellExec, ClassSessionQuery, ClassSessionControl},
	} {
		path, err := ConnectRequestTarget(caps)
		if err != nil {
			t.Fatalf("%v: target: %v", caps, err)
		}
		if _, err := HTTPBaseString(now, nonce, http.MethodGet, path, nil); err != nil {
			t.Fatalf("%v: negotiated declaration refused: %v", caps, err)
		}
	}
	prefix := ConnectPath + ConnectVersionQueryPrefix + ContractVersion + ConnectCapabilitiesQuery
	for _, bad := range []string{
		prefix,
		prefix + "notify_local,machine_state_query",
		prefix + "shell_exec,shell_exec",
		prefix + "shell_exec,unknown_class",
		prefix + "shell_exec,",
		prefix + "shell_exec&extra=1",
		prefix + "shell%5Fexec",
		ConnectPath + ConnectVersionQueryPrefix + "0.2.3" + ConnectCapabilitiesQuery + "machine_state_query",
	} {
		if _, err := HTTPBaseString(now, nonce, http.MethodGet, bad, nil); err == nil {
			t.Fatalf("malformed declaration signed: %q", bad)
		}
	}
}
