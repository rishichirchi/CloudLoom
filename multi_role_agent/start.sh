#!/bin/bash

# Startup script for Sentinel AI Multi-Role Agent

echo "🚀 Starting Sentinel AI Multi-Role Agent..."

# Check if virtual environment exists
if [ ! -d "venv" ]; then
    echo "❌ Virtual environment not found. Please run setup first."
    exit 1
fi

# Activate virtual environment
source venv/bin/activate

# Check if .env file exists
if [ ! -f ".env" ]; then
    echo "⚠️  .env file not found. Please copy .env.example to .env and configure your Google Gemini API settings."
    echo "   cp .env.example .env"
    echo "   # Then edit .env with your Google API key"
    exit 1
fi

# Check if logs.json exists (required by the application)
if [ ! -f "logs.json" ]; then
    echo "📝 Creating default logs.json file..."
    cat > logs.json << 'EOF'
{
    "logs": [
        {
            "timestamp": "2025-01-01T00:00:00Z",
            "level": "INFO",
            "message": "System initialized",
            "source": "application"
        }
    ]
}
EOF
fi

echo "✅ Environment ready. Starting FastAPI server..."
echo "📡 Server will be available at: http://localhost:8000"
echo "📚 API docs will be available at: http://localhost:8000/docs"
echo "🛑 Press Ctrl+C to stop the server"

# Start the FastAPI application
uvicorn agent:app --host 0.0.0.0 --port 8000 --reload
