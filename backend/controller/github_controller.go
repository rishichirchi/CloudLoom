package controller

import (
	"context"
	"fmt"
	"io/ioutil"

	// "fmt"
	"net/http"
	"github.com/rishichirchi/cloudloom/models"
	githubsvc "github.com/rishichirchi/cloudloom/services/github"
	"strings"

	"github.com/gin-gonic/gin"
	github "github.com/google/go-github/v53/github"
)

type PRRequest struct {
	FilePath    string `json:"file_path"`
	FileContent string `json:"file_content"`
}

func TraceHandler(c *gin.Context) {
	var traceRequest models.TraceRequest
	if err := c.ShouldBindJSON(&traceRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	go processMisConfig(c, traceRequest)

}

func GitHubIWebhook(c *gin.Context) {
	// Parse the request body
	var githubIWebhook models.GitHubIWebhook
	if err := c.BindJSON(&githubIWebhook); err != nil {
		fmt.Println("Error binding JSON:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	installationId := githubIWebhook.Installation.ID
	repoFullName := githubIWebhook.Repository.FullName
	// You can now use the installationId and repoFullName to perform actions

	fmt.Println("Installation ID:", installationId)
	fmt.Println("Repository Full Name:", repoFullName)

}

func GetIacContent(c *gin.Context) {
	getIaCFileContent(c)
}

func processMisConfig(c *gin.Context, req models.TraceRequest) {
	fmt.Println("Reached")
	client, _ := githubsvc.GetGHClient(0000000, 0000000)
	fmt.Println("Client:", client)
	//find the pr
	prs, _, err := client.PullRequests.ListFiles(c, "Somnathumapathi", "CraveHub", 10, nil)
	if err != nil {
		fmt.Println("Error listing pull requests:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, pr := range prs {
		fmt.Println("PR:", pr)
	}

}

func getIaCFileContent(c *gin.Context) {

	client, err := githubsvc.GetGHClient(int64(67221597), int64(1271564)) // Use actual installation/account IDs
	if err != nil || client == nil {
		fmt.Printf("Error getting GitHub client: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize GitHub client"})
		return
	}
	prs, err := getPrs(c)
	if err != nil {
		prs = make(map[int][]string)
	}

	// Get logs from external URL, suppress error if any
	logs := ""
	resp, err := http.Get("https://119f-2409-40f2-1023-9d6a-efb3-b133-9213-3696.ngrok-free.app/event")
	if err == nil && resp != nil {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err == nil {
			logs = string(body)
		}
	}

	tfFiles := collectIaCFiles(c, client, "rishichirchi", "IaC", "", []string{".tf"})

	// Assuming only one .tf file is present
	for path, content := range tfFiles {
		c.JSON(http.StatusOK, gin.H{
			"path":    path,
			"content": content,
			"prs":     prs,
			"logs":    logs,
		})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"message": "No Terraform files found"})
}

func collectIaCFiles(ctx *gin.Context, client *github.Client, owner, repo, path string, extensions []string) map[string]string {
	results := make(map[string]string)

	fileContent, dirContents, _, err := client.Repositories.GetContents(ctx, owner, repo, path, nil)
	if err != nil {
		fmt.Printf("Error getting contents at path %s: %v\n", path, err)
		return results
	}

	if dirContents != nil {
		for _, content := range dirContents {
			if content == nil {
				continue
			}
			switch content.GetType() {
			case "file":
				for _, ext := range extensions {
					if strings.HasSuffix(content.GetPath(), ext) {
						decoded, err := getDecodedFileContent(ctx, client, owner, repo, content.GetPath())
						if err != nil {
							fmt.Printf("Error decoding %s: %v\n", content.GetPath(), err)
							continue
						}
						results[content.GetPath()] = decoded
					}
				}
			case "dir":
				subResults := collectIaCFiles(ctx, client, owner, repo, content.GetPath(), extensions)
				for k, v := range subResults {
					results[k] = v
				}
			}
		}
	} else if fileContent != nil {
		for _, ext := range extensions {
			if strings.HasSuffix(fileContent.GetPath(), ext) {
				decoded, err := fileContent.GetContent()
				if err != nil {
					fmt.Printf("Error decoding %s: %v\n", fileContent.GetPath(), err)
					break
				}
				results[fileContent.GetPath()] = decoded
			}
		}
	}

	return results
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func getPrs(c *gin.Context) (result map[int][]string, err error) {
	fmt.Println("Reached")
	client, _ := githubsvc.GetGHClient(int64(67221597), int64(1271564))
	fmt.Println("Client:", client)

	owner := "rishichirchi"
	repo := "IaC"

	// List all open pull requests
	prs, _, err := client.PullRequests.List(c, owner, repo, &github.PullRequestListOptions{State: "open"})
	if err != nil {
		fmt.Println("Error listing pull requests:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result = make(map[int][]string) // PR number -> list of .tf files

	for _, pr := range prs {
		files, _, err := client.PullRequests.ListFiles(c, owner, repo, pr.GetNumber(), nil)
		if err != nil {
			fmt.Printf("Error listing files for PR #%d: %v\n", pr.GetNumber(), err)
			continue
		}
		for _, file := range files {
			if strings.HasSuffix(file.GetFilename(), ".tf") {
				result[pr.GetNumber()] = append(result[pr.GetNumber()], file.GetFilename())
			}
		}
	}

	return result, nil
}

func getDecodedFileContent(ctx *gin.Context, client *github.Client, owner, repo, filePath string) (string, error) {
	fileContent, _, _, err := client.Repositories.GetContents(ctx, owner, repo, filePath, nil)
	if err != nil {
		return "", err
	}

	if fileContent == nil {
		return "", fmt.Errorf("file content is nil for path: %s", filePath)
	}

	decoded, err := fileContent.GetContent()
	if err != nil {
		return "", err
	}

	return decoded, nil
}

func createPullRequest(ctx *gin.Context, client *github.Client, owner, repo, headBranch, baseBranch string) {
	newPR := &github.NewPullRequest{
		Title:               github.String("Add Terraform files scan"),
		Head:                github.String(headBranch), // branch where your changes are
		Base:                github.String(baseBranch), // branch you want to merge into
		Body:                github.String("This PR adds Terraform scan results for IaC security review."),
		MaintainerCanModify: github.Bool(true),
	}

	pr, _, err := client.PullRequests.Create(ctx, owner, repo, newPR)

	if err != nil {
		fmt.Printf("Error creating pull request: %v\n", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create pull request"})
		return
	}

	fmt.Printf("Pull request created: %s\n", pr.GetHTMLURL())
	ctx.JSON(http.StatusOK, gin.H{"message": "Pull request created", "url": pr.GetHTMLURL()})
}
func CreatePRHandler(c *gin.Context) {
	var req PRRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	client, err := githubsvc.GetGHClient(int64(67221597), int64(1271564))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "GitHub client error"})
		return
	}
	if req.FilePath == "" {
		req.FilePath = "main.tf"
	}
	owner := "rishichirchi"
	repo := "IaC"
	base := "main"
	newBranch := "fix-iac"
	filePath := req.FilePath
	fileContent := req.FileContent

	ctx := c.Request.Context()

	// Step 1: Create branch if it doesn't exist
	err = createBranch(client, ctx, owner, repo, newBranch, base)
	if err != nil && !strings.Contains(err.Error(), "Reference already exists") {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Step 2: Commit file to branch
	err = commitFileToBranch(client, ctx, owner, repo, newBranch, filePath, fileContent)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Step 3: Create PR
	createPullRequest(c, client, owner, repo, newBranch, base)
}

func createBranch(client *github.Client, ctx context.Context, owner, repo, newBranch, baseBranch string) error {
	// Get the reference to the base branch (usually main)
	baseRef, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+baseBranch)
	if err != nil {
		return fmt.Errorf("failed to get base branch ref: %v", err)
	}

	// Create new reference (branch)
	newRef := &github.Reference{
		Ref: github.String("refs/heads/" + newBranch),
		Object: &github.GitObject{
			SHA: baseRef.Object.SHA,
		},
	}
	_, _, err = client.Git.CreateRef(ctx, owner, repo, newRef)
	if err != nil {
		return fmt.Errorf("failed to create new branch: %v", err)
	}
	return nil
}

func commitFileToBranch(client *github.Client, ctx context.Context, owner, repo, branch, path, content string) error {
	// Get the repo
	repository, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return err
	}
	fmt.Println("Repository:", repository)
	// Get the branch

	// Get current tree
	baseRef, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
	if err != nil {
		return err
	}
	baseCommit, _, err := client.Git.GetCommit(ctx, owner, repo, *baseRef.Object.SHA)
	if err != nil {
		return err
	}

	// Create a blob (file content)
	blob := &github.Blob{
		Content:  github.String(content),
		Encoding: github.String("utf-8"),
	}
	blobRes, _, err := client.Git.CreateBlob(ctx, owner, repo, blob)
	if err != nil {
		return err
	}

	// Create a tree
	entry := &github.TreeEntry{
		Path: github.String(path),
		Mode: github.String("100644"),
		Type: github.String("blob"),
		SHA:  blobRes.SHA,
	}
	tree, _, err := client.Git.CreateTree(ctx, owner, repo, *baseCommit.Tree.SHA, []*github.TreeEntry{entry})
	if err != nil {
		return err
	}

	// Create a commit
	newCommit := &github.Commit{
		Message: github.String("Add scanned IaC file"),
		Tree:    tree,
		Parents: []*github.Commit{baseCommit},
	}
	commit, _, err := client.Git.CreateCommit(ctx, owner, repo, newCommit)
	if err != nil {
		return err
	}

	// Update branch to point to new commit
	baseRef.Object.SHA = commit.SHA
	_, _, err = client.Git.UpdateRef(ctx, owner, repo, baseRef, false)
	return err
}
