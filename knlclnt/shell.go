package main

import (
	"context"
	"log"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"
	"kubenetlab.net/knl/api/v1beta1"
)

func (cli *CLI) ShellNode(cmd *cobra.Command, args []string) {
	clnt, err := cli.getClnt()
	if err != nil {
		log.Fatal(err)
	}
	lab := &v1beta1.Lab{}
	labKey := types.NamespacedName{Namespace: cli.Namespace, Name: cli.Shell.Lab}
	err = clnt.Get(context.Background(), labKey, lab)
	if err != nil {
		log.Fatal(err)
	}
	sys, _ := lab.Spec.NodeList[cli.Shell.Node].GetSystem()
	sys.Shell(context.Background(), clnt, cli.Namespace, cli.Shell.Lab, cli.Shell.Node, "")
}
