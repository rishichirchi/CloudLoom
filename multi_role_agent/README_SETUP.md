# Sentinel AI - Multi-Role Agent

This project is a multi-role AI agent system that analyzes Terraform infrastructure code for security vulnerabilities and provides recommendations.

## Features

- **Task Planning Agent**: Breaks down complex requests into manageable tasks
- **Cloud AI Agent**: Executes tasks using various tools including file operations, script execution, and documentation retrieval
- **Security Analysis**: Analyzes Terraform code for security vulnerabilities
- **Graph Generation**: Creates Mermaid.js graphs representing infrastructure resources
- **Vulnerability Remediation**: Generates fixed Terraform code addressing security issues

## Prerequisites

- Python 3.12 or higher
- Google Gemini API key

## Setup Instructions

### 1. Virtual Environment Setup

The virtual environment has already been created and configured with all necessary dependencies.

### 2. Environment Configuration

1. Copy the example environment file:
   ```bash
   cp .env.example .env
   ```

2. Edit the `.env` file and add your Google Gemini API key:
   ```bash
   # Google Gemini Configuration
   GOOGLE_API_KEY=your_actual_google_gemini_api_key_here
   ```

   To get a Google Gemini API key:
   - Visit [Google AI Studio](https://makersuite.google.com/app/apikey)
   - Sign in with your Google account
   - Click "Create API Key"
   - Copy the generated key and paste it in your `.env` file

### 3. Running the Application

Start the FastAPI server:
```bash
./start.sh
```

Or manually:
```bash
source venv/bin/activate
uvicorn agent:app --host 0.0.0.0 --port 8000 --reload
```

The application will be available at:
- **API Server**: http://localhost:8000
- **API Documentation**: http://localhost:8000/docs
- **Alternative API Docs**: http://localhost:8000/redoc

## API Endpoints

### POST /process_terraform/

Processes Terraform code for security analysis and generates:
- Original infrastructure graph (Mermaid.js)
- Security vulnerability report (JSON format)
- Fixed Terraform code
- Updated infrastructure graph

**Response Format:**
```json
{
  "report": "Security vulnerability analysis in JSON format",
  "original_graph": "Mermaid.js code for original infrastructure",
  "changed_graph": "Mermaid.js code for fixed infrastructure", 
  "changed_terraform": "Fixed Terraform code",
  "original_terraform": "Original Terraform code"
}
```

## Project Structure

```
├── agent.py              # Main application with FastAPI server and AI agents
├── pyproject.toml         # Project dependencies
├── .env.example          # Environment configuration template
├── .env                  # Your actual environment configuration (create this)
├── start.sh              # Startup script
├── test_imports.py       # Import validation script
├── venv/                 # Virtual environment
└── index/                # Chroma vector database for documentation retrieval
```

## Dependencies

Key dependencies include:
- **FastAPI**: Web framework for the API
- **LangChain**: AI agent framework
- **Google Gemini**: LLM for natural language processing
- **ChromaDB**: Vector database for document retrieval
- **Pydantic**: Data validation
- **Python-dotenv**: Environment variable management

## Troubleshooting

1. **Import Errors**: Run `python test_imports.py` to verify all dependencies are installed correctly.

2. **API Key Issues**: Ensure your Google Gemini API key is correctly set in the `.env` file.

3. **Port Conflicts**: If port 8000 is in use, modify the port in `start.sh` or run manually with a different port:
   ```bash
   uvicorn agent:app --host 0.0.0.0 --port 8080 --reload
   ```

4. **Missing logs.json**: The application will automatically create a default `logs.json` file if it doesn't exist.

## Development

To add new tools or modify the agent behavior:
1. Edit the tool definitions in `agent.py`
2. Update the agent prompts as needed
3. Restart the server to see changes (auto-reload is enabled)

## Security Notes

- Keep your Google Gemini API key secure and never commit it to version control
- The application restricts file operations to the current directory for security
- Review and validate any generated Terraform code before applying it to your infrastructure
