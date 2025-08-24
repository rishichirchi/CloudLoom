package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"time"

	"github.com/gin-gonic/gin"
)

func GetLiveInfrastructureData(c *gin.Context) {
	log.Println("Executing Steampipe data export script...")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "./infra/live-aws-infra/generate_infra_data.sh")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("Script execution timed out after 5 minutes")
			c.JSON(408, gin.H{"error": "Script execution timed out"})
			return
		}
		log.Printf("Script execution failed. Output:\n%s", string(output))
		c.JSON(500, gin.H{"error": "Failed to retrieve infrastructure data"})
		return
	}

	log.Printf("Script executed successfully. Output:\n%s", string(output))
	c.JSON(200, gin.H{"data": string(output)})
}

type InfrastructureInput struct {
	InfrastructureData map[string]interface{} `json:"infrastructure_data"`
	TerraformState     map[string]interface{} `json:"terraform_state"`
}

type DiagramResponse struct {
	InfrastructureDiagram string `json:"infrastructure_diagram"`
	SecurityDiagram       string `json:"security_diagram"`
	AgentOutput           string `json:"agent_output"`
	Status                string `json:"status"`
	FileSaved             string `json:"file_saved,omitempty"`
	Error                 string `json:"error,omitempty"`
}

type MermaidDiagramResponse struct {
	MermaidCode           string `json:"mermaid_code"`
	SecurityMermaidCode   string `json:"security_mermaid_code,omitempty"`
	DiagramType           string `json:"diagram_type"`
	Status                string `json:"status"`
	GeneratedFiles        []string `json:"generated_files"`
	Error                 string `json:"error,omitempty"`
}

func GenerateInfrastructureDiagram(c *gin.Context) {
	log.Println("Generating infrastructure diagram...")

	// Read infrastructure data from the generated file
	infraData, err := ioutil.ReadFile("infrastructure_data.json")
	if err != nil {
		log.Printf("Failed to read infrastructure_data.json: %v", err)
		c.JSON(500, gin.H{"error": "Failed to read infrastructure data"})
		return
	}

	// Read terraform state data
	terraformData, err := ioutil.ReadFile("infra/iac/terraform.tfstate")
	if err != nil {
		log.Printf("Failed to read terraform.tfstate: %v", err)
		c.JSON(500, gin.H{"error": "Failed to read terraform state"})
		return
	}

	// Parse JSON data
	var infraJSON map[string]interface{}
	if err := json.Unmarshal(infraData, &infraJSON); err != nil {
		log.Printf("Failed to parse infrastructure JSON: %v", err)
		c.JSON(500, gin.H{"error": "Failed to parse infrastructure data"})
		return
	}

	var terraformJSON map[string]interface{}
	if err := json.Unmarshal(terraformData, &terraformJSON); err != nil {
		log.Printf("Failed to parse terraform JSON: %v", err)
		c.JSON(500, gin.H{"error": "Failed to parse terraform state"})
		return
	}

	// Prepare request payload for the Python agent
	requestPayload := InfrastructureInput{
		InfrastructureData: infraJSON,
		TerraformState:     terraformJSON,
	}

	jsonPayload, err := json.Marshal(requestPayload)
	if err != nil {
		log.Printf("Failed to marshal request payload: %v", err)
		c.JSON(500, gin.H{"error": "Failed to prepare request data"})
		return
	}

	// Make HTTP request to Python agent
	agentURL := "http://localhost:8001/generate_infrastructure_diagram/"
	req, err := http.NewRequest("POST", agentURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		c.JSON(500, gin.H{"error": "Failed to create agent request"})
		return
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to call Python agent: %v", err)
		c.JSON(500, gin.H{"error": "Failed to connect to AI agent"})
		return
	}
	defer resp.Body.Close()

	// Read response
	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read agent response: %v", err)
		c.JSON(500, gin.H{"error": "Failed to read agent response"})
		return
	}

	// Parse response
	var diagramResponse DiagramResponse
	if err := json.Unmarshal(responseBody, &diagramResponse); err != nil {
		log.Printf("Failed to parse agent response: %v", err)
		c.JSON(500, gin.H{"error": "Failed to parse agent response"})
		return
	}

	if resp.StatusCode != 200 {
		log.Printf("Agent returned error: %s", diagramResponse.Error)
		c.JSON(resp.StatusCode, gin.H{"error": diagramResponse.Error})
		return
	}

	log.Println("Infrastructure diagram generated successfully")
	c.JSON(200, diagramResponse)
}

// GetMermaidDiagramCode returns clean Mermaid code ready for direct use
func GetMermaidDiagramCode(c *gin.Context) {
	log.Println("Retrieving clean Mermaid diagram code...")

	// First, trigger the diagram generation
	err := triggerDiagramGeneration()
	if err != nil {
		log.Printf("Failed to generate diagrams: %v", err)
		c.JSON(500, gin.H{"error": "Failed to generate diagrams"})
		return
	}

	// Read the generated Mermaid files directly from disk
	var generatedFiles []string
	var mermaidCode, securityMermaidCode string

	// Read infrastructure diagram
	if infraCode, err := readCleanMermaidFile("../multi_role_agent/infrastructure_diagram.txt"); err == nil {
		mermaidCode = infraCode
		generatedFiles = append(generatedFiles, "infrastructure_diagram.txt")
	} else {
		log.Printf("Warning: Could not read infrastructure diagram: %v", err)
	}

	// Read security diagram if it exists
	if secCode, err := readCleanMermaidFile("../multi_role_agent/security_relationship_graph.txt"); err == nil {
		securityMermaidCode = secCode
		generatedFiles = append(generatedFiles, "security_relationship_graph.txt")
	} else {
		log.Printf("Warning: Could not read security diagram: %v", err)
	}

	if mermaidCode == "" {
		c.JSON(500, gin.H{"error": "No valid Mermaid diagrams were generated"})
		return
	}

	response := MermaidDiagramResponse{
		MermaidCode:         mermaidCode,
		SecurityMermaidCode: securityMermaidCode,
		DiagramType:         "infrastructure",
		Status:              "success",
		GeneratedFiles:      generatedFiles,
	}

	log.Printf("Successfully retrieved clean Mermaid code (%d chars)", len(mermaidCode))
	c.JSON(200, response)
}

// Helper function to trigger diagram generation
func triggerDiagramGeneration() error {
	// Read infrastructure data
	infraData, err := ioutil.ReadFile("infrastructure_data.json")
	if err != nil {
		return err
	}

	terraformData, err := ioutil.ReadFile("infra/iac/terraform.tfstate")
	if err != nil {
		return err
	}

	var infraJSON, terraformJSON map[string]interface{}
	json.Unmarshal(infraData, &infraJSON)
	json.Unmarshal(terraformData, &terraformJSON)

	requestPayload := InfrastructureInput{
		InfrastructureData: infraJSON,
		TerraformState:     terraformJSON,
	}

	jsonPayload, _ := json.Marshal(requestPayload)

	// Call both endpoints to generate files
	endpoints := []string{
		"http://localhost:8001/generate_infrastructure_diagram/",
		"http://localhost:8001/generate_security_graph/",
	}

	for _, endpoint := range endpoints {
		req, _ := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Minute}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Warning: Failed to call %s: %v", endpoint, err)
			continue
		}
		resp.Body.Close()
	}

	return nil
}

// Helper function to read and clean Mermaid files
func readCleanMermaidFile(filePath string) (string, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	// Clean the content to ensure it's valid Mermaid
	cleanContent := cleanMermaidCode(string(content))
	return cleanContent, nil
}

// Helper function to clean Mermaid code for proper rendering
func cleanMermaidCode(input string) string {
	// Remove any remaining escape characters
	cleaned := input
	
	// Remove literal \n, \t, \" sequences
	cleaned = bytes.NewBuffer([]byte(cleaned)).String()
	
	// Ensure proper line endings
	cleaned = string(bytes.ReplaceAll([]byte(cleaned), []byte("\r\n"), []byte("\n")))
	cleaned = string(bytes.ReplaceAll([]byte(cleaned), []byte("\r"), []byte("\n")))
	
	// Remove any remaining markdown fences
	lines := bytes.Split([]byte(cleaned), []byte("\n"))
	var result [][]byte
	
	inCodeBlock := false
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("```")) {
			inCodeBlock = !inCodeBlock
			continue
		}
		if !inCodeBlock || !bytes.HasPrefix(trimmed, []byte("```")) {
			result = append(result, line)
		}
	}
	
	finalContent := string(bytes.Join(result, []byte("\n")))
	
	// Ensure it starts with graph declaration
	if !bytes.Contains([]byte(finalContent), []byte("graph")) {
		finalContent = "graph TD\n" + finalContent
	}
	
	return finalContent
}
