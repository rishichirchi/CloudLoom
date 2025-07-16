#!/usr/bin/env python3
"""
Test script to verify all required imports for the agent.py file
"""

print("Testing imports...")

try:
    import os
    print("✓ os")
    
    import json
    print("✓ json")
    
    import re
    print("✓ re")
    
    from typing import List, Dict, Any, Optional, Type
    print("✓ typing")
    
    from pydantic import BaseModel, Field
    print("✓ pydantic")
    
    # LangChain imports
    from langchain_huggingface import HuggingFaceEmbeddings
    print("✓ langchain_huggingface")
    
    from langchain_google_genai import ChatGoogleGenerativeAI
    print("✓ langchain_google_genai")
    
    from langchain.agents import AgentExecutor, create_openai_tools_agent
    print("✓ langchain.agents")
    
    from langchain_core.prompts import ChatPromptTemplate, MessagesPlaceholder
    print("✓ langchain_core.prompts")
    
    from langchain_core.tools import BaseTool, tool
    print("✓ langchain_core.tools")
    
    from langchain_community.tools.ddg_search import DuckDuckGoSearchRun
    print("✓ langchain_community.tools.ddg_search")
    
    from langchain.docstore.document import Document
    print("✓ langchain.docstore.document")
    
    from langchain.tools.retriever import create_retriever_tool
    print("✓ langchain.tools.retriever")
    
    from langchain_community.tools.file_management import ReadFileTool, WriteFileTool
    print("✓ langchain_community.tools.file_management")
    
    from langchain_community.tools import ShellTool
    print("✓ langchain_community.tools")
    
    from langchain_chroma import Chroma
    print("✓ langchain_chroma")
    
    from dotenv import load_dotenv
    print("✓ dotenv")
    
    # FastAPI imports
    from fastapi import FastAPI
    print("✓ fastapi")
    
    from fastapi.responses import JSONResponse
    print("✓ fastapi.responses")
    
    from fastapi.middleware.cors import CORSMiddleware
    print("✓ fastapi.middleware.cors")
    
    import requests
    print("✓ requests")
    
    print("\n🎉 All imports successful! The environment is ready.")
    
except ImportError as e:
    print(f"❌ Import error: {e}")
    print("Some dependencies may be missing.")
    exit(1)
except Exception as e:
    print(f"❌ Unexpected error: {e}")
    exit(1)
