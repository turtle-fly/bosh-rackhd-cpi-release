package onrackhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/onrack/onrack-cpi/config"
)

func PublishWorkflow(c config.Cpi, workflowBytes []byte) error {
	url := fmt.Sprintf("http://%s:8080/api/1.1/workflows", c.ApiServer)

	request, err := http.NewRequest("PUT", url, bytes.NewReader(workflowBytes))
	if err != nil {
		log.Printf("error building http request: %s\n", err)
		return fmt.Errorf("error building http request: %s", err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Printf("error sending publishing workflow to %s\n", url)
		return fmt.Errorf("error sending publishing workflow to %s", url)
	}
	defer resp.Body.Close()

	msg, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error reading response body: %s\n", err)
		return fmt.Errorf("error reading response body: %s", err)
	}

	if resp.StatusCode != 200 {
		log.Printf("error response code is %d: %s\n", resp.StatusCode, string(msg))
		return fmt.Errorf("Failed publishing workflows with status: %s", resp.Status)
	}

	workflowStub := WorkflowStub{}
	err = json.Unmarshal(workflowBytes, &workflowStub)
	if err != nil {
		log.Printf("error unmarshalling workflow: %s\n", err)
		return fmt.Errorf("error unmarshalling workflow: %s\n", err)
	}

	publishedWorkflowsBytes, err := RetrieveWorkflows(c)
	if err != nil {
		return err
	}

	publishedWorkflows := []WorkflowStub{}
	err = json.Unmarshal(publishedWorkflowsBytes, &publishedWorkflows)
	if err != nil {
		log.Printf("error unmarshalling published workflows: %s\n", err)
		return fmt.Errorf("error unmarshalling published workflows: %s\n", err)
	}

	var uploadedWorkflow *WorkflowStub
	for i := range publishedWorkflows {
		if publishedWorkflows[i].Name == workflowStub.Name {
			uploadedWorkflow = &publishedWorkflows[i]
		}
	}

	if uploadedWorkflow == nil {
		log.Println("workflow was not successfully uploaded to server")
		return errors.New("workflow was not successfully uploaded to server")
	}

	return nil
}

func RetrieveWorkflows(c config.Cpi) ([]byte, error) {
	url := fmt.Sprintf("http://%s:8080/api/1.1/workflows/library", c.ApiServer)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error: %s\n", err)
		return nil, fmt.Errorf("Error: %s", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error reading response body: %s\n", err)
		return nil, fmt.Errorf("error reading response body: %s", err)
	}

	if resp.StatusCode != 200 {
		log.Printf("error retrieving tasks: response code is %d: %s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("Failed retrieving workflows with status: %s, message: %s", resp.Status, string(body))
	}

	return body, nil
}

func RunWorkflow(c config.Cpi, nodeID string, req RunWorkflowRequestBody) (err error) {
	url := fmt.Sprintf("http://%s:8080/api/1.1/nodes/%s/workflows/", c.ApiServer, nodeID)
	body, err := json.Marshal(req)
	if err != nil {
		log.Printf("error marshalling workflow request body")
		return
	}
	request, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("error building http request to run workflow")
		return
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Printf("error running workflow at url %s", url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		msg, _ := ioutil.ReadAll(resp.Body)
		log.Printf("error response code is %d: %s, with body: %s", resp.StatusCode, string(msg), string(body))
		return fmt.Errorf("Failed running workflow at url: %s with status: %s, message: %s", url, resp.Status, string(msg))
	}

	wfRespBytes, err := ioutil.ReadAll(resp.Body)
	log.Printf("body response is %s", string(wfRespBytes))
	if err != nil {
		log.Printf("error reading workflow response body %s", err)
		return fmt.Errorf("error reading workflow response body %s", err)
	}

	workflowResp := WorkflowResponse{}
	err = json.Unmarshal(wfRespBytes, &workflowResp)
	if err != nil {
		log.Printf("error unmarshalling /common/node/workflows response %s", err)
		return fmt.Errorf("error unmarshalling /common/node/workflows response %s", err)
	}

	log.Printf("workflow response %v", workflowResp)
	timeoutChan := time.NewTimer(time.Second * c.RunWorkflowTimeoutSeconds).C
	retryChan := time.NewTicker(time.Millisecond * 750).C

	for {
		select {
		case <-timeoutChan:
			err := killActiveWorkflows(c, nodeID)
			if err != nil {
				log.Printf("Could not abort timed out workflow on node: %s\n", nodeID)
				return fmt.Errorf("Could not abort timed out workflow on node: %s", nodeID)
			}

			log.Printf("Timed out running workflow: %s on node: %s", req.Name, nodeID)
			return fmt.Errorf("Timed out running workflow: %s on node: %s", req.Name, nodeID)
		case <-retryChan:
			status, err := getWorkflowStatus(c, nodeID, workflowResp.ID)
			if err != nil {
				log.Printf("Unable to fetch workflow status: %s\n", err)
				return err
			}

			switch status {
			case workflowValidStatus:
				log.Printf("workflow: %s is still running against node: %s", req.Name, nodeID)
				continue
			case worfklowSuccessfulStatus:
				log.Printf("workflow: %s completed successfully against node: %s", req.Name, nodeID)
				return nil
			case workflowFailedStatus:
				log.Printf("workflow: %s failed against node: %s", req.Name, nodeID)
				return fmt.Errorf("workflow: %s failed against node: %s", req.Name, nodeID)
			case workflowCancelledStatus:
				log.Printf("workflow: %s was cancelled against node: %s", req.Name, nodeID)
				return nil
			default:
				log.Printf("workflow: %s has unexpected status on node: %s", req.Name, nodeID)
				return fmt.Errorf("workflow: %s has unexpected status %s on node: %s", req.Name, status, nodeID)
			}
		}
	}
}

func killActiveWorkflows(c config.Cpi, nodeID string) error {
	url := fmt.Sprintf("http://%s:8080/api/1.1/nodes/%s/workflows/active", c.ApiServer, nodeID)
	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		log.Printf("error: %s  building http request to delete active workflows against node: %s\n", err, nodeID)
		return fmt.Errorf("error: %s building http request to delete active workflows against node: %s", err, nodeID)
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Printf("Error: %s deleting active workflows on node: %s\n", err, nodeID)
		return fmt.Errorf("Error: %s deleting active workflows on node: %s", err, nodeID)
	}

	defer resp.Body.Close()

	msg, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error: %s reading response body from abort workflows request against node: %s\n", err, nodeID)
		return fmt.Errorf("Error: %s reading response body from abort workflows request against node: %s", err, nodeID)
	}

	if resp.StatusCode != 200 {
		log.Printf("Failed deleting active workflows against node: %s with status: %s, message: %s", nodeID, resp.Status, string(msg))
		return fmt.Errorf("Failed deleting active workflows against node: %s with status: %s, message: %s", nodeID, resp.Status, string(msg))
	}

	return nil
}

func getActiveWorkflows(c config.Cpi, nodeID string) ([]WorkflowResponse, error) {
	var workflows []WorkflowResponse

	url := fmt.Sprintf("http://%s:8080/api/1.1/nodes/%s/workflows/active", c.ApiServer, nodeID)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error requesting active workflows on node at url: %s, msg: %s", url, err)
		return []WorkflowResponse{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg, _ := ioutil.ReadAll(resp.Body)
		log.Printf("Failed retrieving active workflows at url: %s with status: %s, message: %s", url, resp.Status, string(msg))
		return []WorkflowResponse{}, fmt.Errorf("Failed retrieving active workflows at url: %s with status: %s, message: %s", url, resp.Status, string(msg))
	}

	body, err := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &workflows)
	if err != nil {
		log.Printf("Error unmarshalling active workflows: %s", err)
		return []WorkflowResponse{}, fmt.Errorf("Error unmarshalling active workflows: %s", err)
	}

	return workflows, nil
}

func getWorkflowStatus(c config.Cpi, nodeID string, workflowID string) (string, error) {
	url := fmt.Sprintf("http://%s:8080/api/1.1/nodes/%s/workflows", c.ApiServer, nodeID)
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Error requesting active workflows on node at url: %s, msg: %s", url, err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg, _ := ioutil.ReadAll(resp.Body)
		log.Printf("Failed retrieving active workflows at url: %s with status: %s, message: %s", url, resp.Status, string(msg))
		return "", fmt.Errorf("Failed retrieving active workflows at url: %s with status: %s, message: %s", url, resp.Status, string(msg))
	}

	body, err := ioutil.ReadAll(resp.Body)
	var workflows []WorkflowResponse
	err = json.Unmarshal(body, &workflows)
	if err != nil {
		log.Printf("Error unmarshalling active workflows: %s", err)
		return "", fmt.Errorf("Error unmarshalling active workflows: %s", err)
	}

	var w *WorkflowResponse
	for i := range workflows {
		if workflows[i].ID == workflowID {
			w = &workflows[i]
			if w.Status == workflowValidStatus && len(w.PendingTasks) == 0 {
				return worfklowSuccessfulStatus, nil
			}
		}
	}

	if w == nil {
		log.Printf("could not find workflow with name: %s on node: %s", workflowID, nodeID)
		return "", fmt.Errorf("could not find workflow with name: %s on node: %s", workflowID, nodeID)
	}

	return w.Status, nil
}

const (
	OnrackReserveVMGraphName = "Graph.CF.ReserveVM"
	OnrackCreateVMGraphName  = "Graph.BOSH.ProvisionNode"
	OnrackDeleteVMGraphName  = "Graph.CF.DeleteVM"
	OnrackEnvPath            = "/var/vcap/bosh/onrack-cpi-agent-env.json"
	OnrackRegistryPath       = "/var/vcap/bosh/agent.json"
	DefaultUnusedName        = "UPLOADED_BY_ONRACK_CPI"
)

const (
	workflowValidStatus      = "valid"
	worfklowSuccessfulStatus = "succeeded"
	workflowFailedStatus     = "failed"
	workflowCancelledStatus  = "cancelled"
)

type NodeWorkflow struct {
	NodeID         string `json:"node"`
	InjectableName string `json:"injectableName"`
	Status         string `json:"_status"`
}

type Workflow struct {
	Name       string                 `json:"injectableName"`
	UnusedName string                 `json:"friendlyName"`
	Tasks      []WorkflowTask         `json:"tasks"`
	Options    map[string]interface{} `json:"options"`
}

type WorkflowStub struct {
	Name       string         `json:"injectableName"`
	UnusedName string         `json:"friendlyName"`
	Tasks      []WorkflowTask `json:"tasks"`
}

type WorkflowResponse struct {
	Name   					string                  	`json:"injectableName"`
	Tasks  					map[string]TaskResponse 	`json:"tasks"`
	Status 					string                  	`json:"_status"`
	ID     					string                   	`json:"id"`
	PendingTasks		[]interface{} 						`json:"pendingTasks"`
}

type PropertyContainer struct {
	Properties interface{} `json:"properties"`
}

type OptionContainer struct {
	Options interface{} `json:"options"`
}

type RunWorkflowRequestBody struct {
	Name    string                 `json:"name"`
	Options map[string]interface{} `json:"options"`
}
