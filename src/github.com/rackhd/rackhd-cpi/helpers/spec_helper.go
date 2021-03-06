package helpers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/rackhd/rackhd-cpi/bosh"
	"github.com/rackhd/rackhd-cpi/config"
	"github.com/rackhd/rackhd-cpi/rackhdapi"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

const (
	env_rackhd_api_url = "RACKHD_API_URL"
)

func SetUp(cpiRequestType string) (*ghttp.Server, *strings.Reader, config.Cpi, bosh.CpiRequest) {

	var err error
	server := ghttp.NewServer()
	jsonReader := strings.NewReader(fmt.Sprintf(`{"api_url":"%s", "agent":{"blobstore": {"provider":"local","some": "options"}, "mbus":"localhost"}, "max_reserve_node_attempts":1}`, server.URL()))
	request := bosh.CpiRequest{Method: cpiRequestType}
	cpiConfig, err := config.New(jsonReader, request)
	Expect(err).ToNot(HaveOccurred())

	return server, jsonReader, cpiConfig, request
}

func GetRackHDHost() (string, error) {
	raw_url := os.Getenv(env_rackhd_api_url)
	if raw_url == "" {
		return "", fmt.Errorf("Environment variable %s not set", env_rackhd_api_url)
	}
	return raw_url, nil
}

func LoadJSON(nodePath string) []byte {
	dummyResponseFile, err := os.Open(nodePath)
	Expect(err).ToNot(HaveOccurred())
	defer dummyResponseFile.Close()

	dummyResponseBytes, err := ioutil.ReadAll(dummyResponseFile)
	Expect(err).ToNot(HaveOccurred())

	return dummyResponseBytes
}

func LoadStruct(filePath string, o interface{}) interface{} {
	dummyResponseBytes := LoadJSON(filePath)

	err := json.Unmarshal(dummyResponseBytes, o)
	Expect(err).ToNot(HaveOccurred())

	return o
}

func LoadWorkflow(workflowPath string) rackhdapi.Workflow {
	workflow := rackhdapi.Workflow{}
	return *LoadStruct(workflowPath, &workflow).(*rackhdapi.Workflow)
}

func LoadTask(taskPath string) rackhdapi.Task {
	task := rackhdapi.Task{}
	return *LoadStruct(taskPath, &task).(*rackhdapi.Task)
}

func LoadNodes(nodePath string) []rackhdapi.Node {
	dummyResponseBytes := LoadJSON(nodePath)

	nodes := []rackhdapi.Node{}
	err := json.Unmarshal(dummyResponseBytes, &nodes)
	Expect(err).ToNot(HaveOccurred())

	return nodes
}

func LoadNode(nodePath string) rackhdapi.Node {
	node := rackhdapi.Node{}
	return *LoadStruct(nodePath, &node).(*rackhdapi.Node)
}

func LoadNodeCatalog(nodeCatalogPath string) rackhdapi.NodeCatalog {
	nodeCatalog := rackhdapi.NodeCatalog{}
	return *LoadStruct(nodeCatalogPath, &nodeCatalog).(*rackhdapi.NodeCatalog)
}

func MakeTryReservationHandlers(requestID string, nodeID string, expectedNodesPath string, expectedNodeCatalogPath string) []http.HandlerFunc {
	expectedNodes := LoadNodes(expectedNodesPath)
	expectedNodesData, err := json.Marshal(expectedNodes)
	Expect(err).ToNot(HaveOccurred())
	var expectedNode rackhdapi.Node
	for n := range expectedNodes {
		if expectedNodes[n].ID == nodeID {
			expectedNode = expectedNodes[n]
		}
	}
	Expect(expectedNode).ToNot(BeNil())
	expectedNodeData, err := json.Marshal(expectedNode)
	Expect(err).ToNot(HaveOccurred())
	expectedNodeCatalog := LoadNodeCatalog(expectedNodeCatalogPath)
	expectedNodeCatalogData, err := json.Marshal(expectedNodeCatalog)
	Expect(err).ToNot(HaveOccurred())

	reservationHandlers := []http.HandlerFunc{
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/api/common/nodes"),
			ghttp.RespondWith(http.StatusOK, expectedNodesData),
		),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", fmt.Sprintf("/api/common/nodes/%s/catalogs/ohai", nodeID)),
			ghttp.RespondWith(http.StatusOK, expectedNodeCatalogData),
		),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", fmt.Sprintf("/api/1.1/nodes/%s/workflows/active", nodeID)),
			ghttp.RespondWith(http.StatusOK, nil),
		),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", fmt.Sprintf("/api/common/nodes/%s", nodeID)),
			ghttp.RespondWith(http.StatusOK, expectedNodeData),
		),
	}

	return append(reservationHandlers, MakeWorkflowHandlers("Reserve", requestID, nodeID)...)
}

func MakeWorkflowHandlers(workflow string, requestID string, nodeID string) []http.HandlerFunc {
	taskStubData := []byte(fmt.Sprintf("[{\"injectableName\": \"Task.BOSH.%s.Node.%s\"}]", workflow, requestID))
	workflowStubData := []byte(fmt.Sprintf("[{\"injectableName\": \"Graph.BOSH.%sNode.%s\"}]", workflow, requestID))
	nodeStubData := []byte(`{"obmSettings": [{"service": "fake-obm-service"}]}`)
	completedWorkflowResponse := []byte(fmt.Sprintf("{\"id\": \"%s\", \"_status\": \"succeeded\"}", requestID))

	return []http.HandlerFunc{
		ghttp.VerifyRequest("PUT", "/api/1.1/workflows/tasks"),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/api/1.1/workflows/tasks/library"),
			ghttp.RespondWith(http.StatusOK, taskStubData),
		),
		ghttp.VerifyRequest("PUT", "/api/1.1/workflows"),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/api/1.1/workflows/library"),
			ghttp.RespondWith(http.StatusOK, workflowStubData),
		),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", fmt.Sprintf("/api/common/nodes/%s", nodeID)),
			ghttp.RespondWith(http.StatusOK, nodeStubData),
		),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("POST", fmt.Sprintf("/api/1.1/nodes/%s/workflows/", nodeID)),
			ghttp.RespondWith(http.StatusCreated, completedWorkflowResponse),
		),
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", fmt.Sprintf("/api/common/workflows/%s", requestID)),
			ghttp.RespondWith(http.StatusOK, completedWorkflowResponse),
		),
	}
}
