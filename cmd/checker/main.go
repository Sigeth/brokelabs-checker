// Command checker is the built-in checker: it evaluates the check declared on
// each task of a challenge package. See pkg/sdk for the contract.
package main

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"

	"github.com/Sigeth/brokelabs-core-go/pkg/kubeexec"
	"github.com/Sigeth/brokelabs-core-go/pkg/sshexec"

	"github.com/Sigeth/brokelabs-checker/internal/checks"
	"github.com/Sigeth/brokelabs-checker/pkg/sdk"
)

func main() {
	c := &checks.Checker{SSH: sshexec.New(), Login: sshexec.Login, Kube: checks.Kubeconfig}
	// In the cluster, the operator grants the checker exec into the machines.
	if config, err := rest.InClusterConfig(); err == nil {
		exec, err := kubeexec.New(config)
		if err != nil {
			fmt.Fprintln(os.Stderr, "checker error:", err)
			os.Exit(1)
		}
		c.Exec = exec
	}
	sdk.Main(c.Check)
}
