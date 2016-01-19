package cpi

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/rackhd/rackhd-cpi/config"
	"github.com/rackhd/rackhd-cpi/rackhdapi"
)

func SelectNodeFromRackHD(c config.Cpi, diskCID string) (string, error) {
	nodes, err := rackhdapi.GetNodes(c)
	if err != nil {
		return "", err
	}

	nodeID, err := randomSelectAvailableNode(c, nodes, diskCID)
	if err != nil || nodeID == "" {
		return "", err
	}

	log.Info(fmt.Sprintf("selected node %s", nodeID))
	return nodeID, nil
}

func randomSelectAvailableNode(c config.Cpi, nodes []rackhdapi.Node, diskCID string) (string, error) {
	availableNodes := getAllAvailableNodes(c, nodes, diskCID)
	if len(availableNodes) == 0 {
		return "", errors.New("all nodes have been reserved")
	}

	t := time.Now()
	rand.Seed(t.Unix())

	i := rand.Intn(len(availableNodes))
	return availableNodes[i].ID, nil
}

func getAllAvailableNodes(c config.Cpi, nodes []rackhdapi.Node, diskCID string) []rackhdapi.Node {
	var n []rackhdapi.Node

	for i := range nodes {
		if nodeIsAvailable(c, nodes[i], diskCID) {
			n = append(n, nodes[i])
			log.Debug(fmt.Sprintf("node: %s is avaliable", nodes[i].ID))
		}
	}

	return n
}

func nodeIsAvailable(c config.Cpi, n rackhdapi.Node, diskCID string) bool {
	workflows, _ := rackhdapi.GetActiveWorkflows(c, n.ID)
	obmSettings, _ := rackhdapi.GetOBMSettings(c, n.ID)
	return (n.Status == "" || n.Status == rackhdapi.Available) &&
		(n.CID == "") &&
		(len(workflows) == 0) &&
		(len(obmSettings) > 0) &&
		!hasPersistentDisk(n)
}

func hasPersistentDisk(n rackhdapi.Node) bool {
	return n.PersistentDisk.DiskCID != ""
}
