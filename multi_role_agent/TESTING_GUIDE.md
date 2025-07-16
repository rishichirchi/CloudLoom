# 🧪 Testing the Sentinel AI API with Postman

## Prerequisites ✅

Before testing, ensure you have:

1. **Environment Setup**:
   ```bash
   cd /home/rishi/Desktop/Sentinel-AI/multi_role_agent
   cp .env.example .env
   # Edit .env with your Google Gemini API key
   ```

2. **Required Files** (already exist in your project):
   - `original.tf` - Contains Terraform infrastructure code
   - `logs.json` - Contains CloudTrail logs for analysis

3. **Start the Server**:
   ```bash
   source venv/bin/activate
   python -m uvicorn agent:app --host 0.0.0.0 --port 8000 --reload
   ```

## 🚀 Postman Configuration

### Step 1: Basic Request Setup
- **Method**: `POST`
- **URL**: `http://localhost:8000/process_terraform/`

### Step 2: Headers
```
Content-Type: application/json
Accept: application/json
```

### Step 3: Request Body
**Leave the body EMPTY** - the endpoint reads from local files (`original.tf` and `logs.json`)

### Step 4: Send Request
Click **Send** and wait for the response (this may take 2-5 minutes as AI analyzes the Terraform code)

## 📊 Expected Response

The API will return a JSON object with:

```json
{
  "report": "JSON string containing security vulnerability analysis",
  "original_graph": "Mermaid.js graph code for original infrastructure",
  "changed_graph": "Mermaid.js graph code for remediated infrastructure", 
  "changed_terraform": "Fixed Terraform code with security improvements",
  "original_terraform": "Original Terraform code from original.tf"
}
```

## 🔍 Response Analysis

### 1. Security Report (`report` field)
Contains a detailed JSON analysis of security vulnerabilities like:
- SSH access from anywhere (0.0.0.0/0)
- Unencrypted S3 buckets
- Database publicly accessible
- Hardcoded passwords
- Wide open security groups

### 2. Infrastructure Graphs
- `original_graph`: Mermaid.js visualization of current infrastructure
- `changed_graph`: Mermaid.js visualization after security fixes

### 3. Terraform Code
- `original_terraform`: Your input Terraform code
- `changed_terraform`: Remediated version with security improvements

## 🛠️ Troubleshooting

### Common Issues:

1. **404 Error - File Not Found**:
   ```json
   {"error": "original.tf file not found in the current directory"}
   ```
   **Solution**: Ensure `original.tf` exists in the project directory

2. **500 Error - Google API Key**:
   ```json
   {"error": "Error during agent invocation: ..."}
   ```
   **Solution**: Check your Google Gemini API key in `.env` file

3. **Connection Refused**:
   **Solution**: Make sure the server is running on port 8000

### Quick Health Check:
Test server status with:
- **GET**: `http://localhost:8000/docs` (should show API documentation)

## 📋 Sample Test Workflow

1. **Verify Server**: GET `http://localhost:8000/docs`
2. **Process Terraform**: POST `http://localhost:8000/process_terraform/`
3. **Save Results**: Copy response fields to separate files for analysis
4. **View Graphs**: Paste Mermaid.js code into https://mermaid.live

## 🎯 Success Indicators

✅ **Successful Response**:
- Status: 200 OK
- Response time: 2-5 minutes
- All 5 fields present in JSON response
- `report` contains vulnerability analysis
- `changed_terraform` has security improvements

❌ **Failed Response**:
- Status: 404/500
- Error message in response
- Missing or empty fields

## 📝 Additional Testing

### Test with curl:
```bash
curl -X POST "http://localhost:8000/process_terraform/" \
     -H "Content-Type: application/json" \
     -H "Accept: application/json"
```

### Test with Python:
```python
import requests
import json

response = requests.post("http://localhost:8000/process_terraform/")
if response.status_code == 200:
    result = response.json()
    print("✅ Success!")
    print(f"Report length: {len(result['report'])}")
    print(f"Vulnerabilities found: Check the report field")
else:
    print(f"❌ Error: {response.status_code}")
    print(response.text)
```

## 🔧 Performance Notes

- **Processing Time**: 2-5 minutes (depends on Terraform complexity)
- **Memory Usage**: High during AI processing
- **Output Size**: Can be large (10KB+ for complex infrastructure)

The API processes your Terraform infrastructure through multiple AI agents to:
1. Generate visual graphs
2. Identify security vulnerabilities  
3. Create remediated Terraform code
4. Generate updated graphs

Happy testing! 🚀
