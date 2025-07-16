import os
import json
import re # Added for regex parsing in task planner
from typing import List, Dict, Any, Optional, Type

# Force CPU usage to avoid CUDA out of memory errors
os.environ['CUDA_VISIBLE_DEVICES'] = ''
os.environ['PYTORCH_CUDA_ALLOC_CONF'] = 'max_split_size_mb:32'

from langchain_huggingface import HuggingFaceEmbeddings
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain.agents import AgentExecutor, create_openai_tools_agent # Changed from create_tool_calling_agent
from langchain_core.prompts import ChatPromptTemplate, MessagesPlaceholder
from langchain_core.tools import BaseTool, tool
from pydantic import BaseModel, Field
from langchain_community.tools.ddg_search import DuckDuckGoSearchRun
from langchain.docstore.document import Document
from langchain.tools.retriever import create_retriever_tool
from langchain_community.tools.file_management import ReadFileTool, WriteFileTool
from langchain_community.tools import ShellTool # For execute_script_tool
from  langchain_chroma import Chroma

from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

MODEL_NAME = "gemini-2.0-flash"

# --- Tool Definitions (for CloudAIAgent) ---

# 2. Script Execution Tool
class ExecuteScriptInput(BaseModel):
    script_command: str = Field(description="The shell command to execute the script. E.g., 'python scripts/run_docker.py --arg value'")

@tool("execute_script", args_schema=ExecuteScriptInput)
def execute_script_tool(script_command: str) -> str:
    """
    Executes a pre-approved script using a shell command.
    Only use for scripts that are known and configured.
    Example: 'python scripts/deploy_service.py --config prod_config.json'
    or './scripts/start_container.sh my_image'
    """
    print(f"Executing script: {script_command}")
    try:
        shell_tool_instance = ShellTool()
        return shell_tool_instance.run(script_command)
    except Exception as e:
        return f"Error executing script '{script_command}': {str(e)}"


# 4. Documentation Retrieval Tool
def get_retriever_tool(docs_path: Optional[str] = None, documents_content: Optional[List[str]] = None):
    """
    Creates a retrieval tool from a directory of documents or list of document contents.
    """

    embeddings = HuggingFaceEmbeddings(model_name="intfloat/e5-large-v2")
    try:
        vectorstore = Chroma(persist_directory=docs_path, embedding_function=embeddings)
    except Exception as e:
        print(f"Error creating FAISS vector store: {e}. Ensure 'faiss-cpu' or 'faiss-gpu' is installed.")
        @tool("document_retriever")
        def dummy_retriever_tool_on_error(query: str) -> str:
            """(Error initializing vector store) Retrieves relevant information from indexed documentation."""
            return "Error initializing document retriever."
        return dummy_retriever_tool_on_error

    retriever = vectorstore.as_retriever()
    retriever_tool = create_retriever_tool(
        retriever,
        "document_retriever",
        "Searches and returns relevant excerpts from internal documentation. Use for specific questions about configurations, APIs, or standard procedures."
    )
    return retriever_tool


# 1. all tools including user defined tools for CloudAIAgent
read_file_tool = ReadFileTool()
write_file_tool = WriteFileTool(root_dir=".") # Restrict to current directory for safety
search_tool = DuckDuckGoSearchRun()

# --- Task Planning Agent Definition ---
class Task(BaseModel):
    description: str = Field(description="A clear, concise description of the task to be performed.")
    id: str = Field(description="A unique identifier for the task, e.g., 'task_1_analyze_data'.")

class TaskList(BaseModel):
    tasks: List[Task] = Field(description="A list of tasks to be executed sequentially.")

class TaskPlannerAgent:
    def __init__(self, llm_model_name: str = MODEL_NAME):
        self.llm = ChatGoogleGenerativeAI(
            model=llm_model_name,
            temperature=0,
            google_api_key=os.getenv("GOOGLE_API_KEY")
        )
        self.planner_runnable = self._create_planner_runnable()

    def _create_planner_runnable(self):
        prompt_template = ChatPromptTemplate.from_messages(
            [
                ("system",
                 "You are a task planning assistant. Your goal is to break down a complex user request into a sequence of smaller, manageable tasks. "
                 "Each task should be actionable and contribute to the overall user goal. "
                 "You must respond with a list of tasks structured according to the 'TaskList' format. "
                 "Generate meaningful and unique 'id' for each task, for example 'task_1_analyze_logs', 'task_2_write_report' etc. "
                 "The 'description' should be a clear instruction for another AI agent. If there are instructions common to all tasks, include them in all tasks as weell."
                 "Make sure you don't have unnecessary number of plans. Be very efficient and avoid too many unnecessary steps"
                 "Ensure the tasks are sequential and logical. Output ONLY the JSON structure."),
                ("user", "User Query: {user_query}")
            ]
        )
        try:
            runnable = prompt_template | self.llm.with_structured_output(TaskList)
            # Perform a quick test to ensure with_structured_output is functional
            try:
                test_output = runnable.invoke({"user_query": "Test query: create a file."})
                if isinstance(test_output, TaskList) and isinstance(test_output.tasks, list):
                    print("TaskPlannerAgent: Successfully initialized using with_structured_output.")
                    return runnable
                else:
                    raise ValueError("Test with_structured_output did not return expected TaskList structure.")
            except Exception as e_test:
                print(f"TaskPlannerAgent: Test of with_structured_output failed ({e_test}). Falling back to manual JSON parsing.")
                return prompt_template | self.llm # Fallback
        except AttributeError:
             print("TaskPlannerAgent: `with_structured_output` not available with this LLM/Langchain version. Falling back to manual JSON parsing.")
             return prompt_template | self.llm # Fallback
        except Exception as e:
            print(f"TaskPlannerAgent: Error setting up planner with structured output ({e}). Falling back to manual JSON parsing.")
            return prompt_template | self.llm # Fallback

    def generate_tasks(self, user_query: str) -> List[Dict[str, str]]:
        print(f"\nTaskPlannerAgent: Generating tasks for query: '{user_query}'")
        try:
            response = self.planner_runnable.invoke({"user_query": user_query})

            if isinstance(response, TaskList): # Successfully used with_structured_output
                print("TaskPlannerAgent: Parsed tasks using with_structured_output.")
                return [task.dict() for task in response.tasks]
            else: # Fallback: response is likely an AIMessage, parse its content
                content = response.content if hasattr(response, 'content') else str(response)
                print(f"TaskPlannerAgent: Raw LLM response for tasks (manual parsing): {content}")
                
                # Attempt to extract JSON from potentially messy LLM output
                match = re.search(r"```json\s*([\s\S]*?)\s*```", content)
                if match:
                    json_str = match.group(1).strip()
                else:
                    # If no markdown ```json ```, assume the whole content might be JSON or contain it
                    # Try to find the first '{' or '[' and last '}' or ']'
                    start_brace = content.find('{')
                    start_bracket = content.find('[')
                    
                    if start_brace == -1 and start_bracket == -1:
                        raise ValueError("No JSON object or array found in the response.")

                    if start_brace != -1 and (start_bracket == -1 or start_brace < start_bracket):
                        start_char_pos = start_brace
                        end_char = '}'
                    else:
                        start_char_pos = start_bracket
                        end_char = ']'
                    
                    open_brackets = 0
                    end_char_pos = -1
                    for i in range(start_char_pos, len(content)):
                        if content[i] == ('[' if end_char == ']' else '{'):
                            open_brackets += 1
                        elif content[i] == end_char:
                            open_brackets -=1
                            if open_brackets == 0:
                                end_char_pos = i
                                break
                    if end_char_pos == -1:
                        raise ValueError(f"Could not find matching closing bracket/brace for '{content[start_char_pos]}'.")

                    json_str = content[start_char_pos : end_char_pos+1]

                parsed_data = json.loads(json_str)
                
                task_list_data = []
                if isinstance(parsed_data, dict) and "tasks" in parsed_data and isinstance(parsed_data["tasks"], list):
                    task_list_data = parsed_data["tasks"]
                elif isinstance(parsed_data, list):
                    task_list_data = parsed_data
                else:
                    raise ValueError("Parsed JSON is not a list of tasks or a dict containing a 'tasks' list.")

                validated_tasks = []
                for i, task_item in enumerate(task_list_data):
                    if isinstance(task_item, dict) and "description" in task_item:
                        # Ensure 'id' exists, generate if missing (though prompt asks for it)
                        task_id = task_item.get("id", f"task_{i+1}_generated_id")
                        validated_tasks.append({
                            "description": str(task_item["description"]),
                            "id": str(task_id)
                        })
                    else:
                        print(f"Warning: Skipping invalid task item: {task_item}")
                
                if not validated_tasks and task_list_data: # If all items were invalid but there was data
                     raise ValueError("No valid task items found after validation, though data was parsed.")
                if not validated_tasks: # If list was empty to begin with or after validation
                    raise ValueError("No tasks found in the parsed JSON.")
                print("TaskPlannerAgent: Parsed tasks using manual JSON extraction.")
                return validated_tasks

        except json.JSONDecodeError as e:
            error_msg = f"Error decoding JSON from LLM. Response: {content[:500] if 'content' in locals() else 'N/A'}. Details: {e}"
            print(f"TaskPlannerAgent: {error_msg}")
            return [{"description": error_msg, "id": "task_error_parsing"}]
        except Exception as e:
            error_msg = f"Error generating or processing tasks. Details: {e}"
            print(f"TaskPlannerAgent: {error_msg}")
            return [{"description": error_msg, "id": "task_error_processing"}]


# --- Cloud AI Agent Definition ---
class CloudAIAgent:
    def __init__(self, llm_model_name: str = MODEL_NAME, documentation_contents: Optional[List[str]] = None, documentation_path: Optional[str] = None):
        self.llm = ChatGoogleGenerativeAI(
            model=llm_model_name,
            temperature=0,
            google_api_key=os.getenv("GOOGLE_API_KEY")
        )
        # Note: execute_script_tool is the function object from @tool, not a class instance to be created here.
        self.tools = self._initialize_tools(documentation_contents, documentation_path)
        self.agent_executor = self._create_agent_executor()

    def _initialize_tools(self, documentation_contents: Optional[List[str]] = None, documentation_path: Optional[str] = None) -> List[BaseTool]:
        """Initializes all tools for the agent."""
        retrieval_tool = get_retriever_tool(docs_path=documentation_path, documents_content=documentation_contents)
        return [
            read_file_tool,
            write_file_tool,
            execute_script_tool, # This is the @tool decorated function
            search_tool,
            retrieval_tool,
        ]

    def _create_agent_executor(self) -> AgentExecutor:
        """Creates the agent executor with the LLM, tools, and prompt."""
        prompt = ChatPromptTemplate.from_messages(
            [
                ("system",
                 "You are a helpful and powerful AI cloud operations assistant. "
                 "You have access to various tools to help you. "
                 "Always try to use your document_retriever tool if you are unsure about cloud services, configurations, or best practices before attempting actions like writing files or executing scripts."
                 "You will be given context for each task, including cloud logs, infrastructure graph data, and a memory block."
                 ),
                ("user",
                 "Task: {input}\n\n"
                 "Cloud Logs Snippet:\n{cloud_logs}\n\n"
                 "Infrastructure Graph (e.g., JSON/YAML describing resources):\n{graph_code}\n\n"
                 "Important Memory Block:\n{memory_block}\n\n"
                 "Chat History (if any):\n{chat_history}"
                 ),
                MessagesPlaceholder(variable_name="agent_scratchpad"),
            ]
        )
        agent = create_openai_tools_agent(self.llm, self.tools, prompt)
        return AgentExecutor(agent=agent, tools=self.tools, verbose=True, handle_parsing_errors=True)

    def invoke(self, task: str, cloud_logs: str, graph_code: str, memory_block: str, chat_history: Optional[List[Dict[str,str]]] = None) -> Dict[str, Any]:
        from langchain_core.messages import HumanMessage, AIMessage

        processed_chat_history = []
        if chat_history:
            for msg in chat_history:
                if msg.get("role") == "user":
                    processed_chat_history.append(HumanMessage(content=msg.get("content", "")))
                elif msg.get("role") == "ai" or msg.get("role") == "assistant":
                    processed_chat_history.append(AIMessage(content=msg.get("content", "")))

        inputs = {
            "input": task,
            "cloud_logs": cloud_logs or "N/A",
            "graph_code": graph_code or "N/A",
            "memory_block": memory_block or "N/A",
            "chat_history": processed_chat_history
        }
        try:
            response = self.agent_executor.invoke(inputs)
            return response
        except Exception as e:
            print(f"Error during agent invocation: {e}")
            return {"output": f"An error occurred: {str(e)}"}

from fastapi import FastAPI
from fastapi.responses import JSONResponse
from fastapi.middleware.cors import CORSMiddleware
import requests

app = FastAPI( title="Multi-Role AI Agent API", description="API for Multi-Role AI Agent", version="1.0")
# CORS middleware to allow requests from any origin
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

class TerraformCode(BaseModel):
    terraform_code: str

@app.post("/process_terraform/")
async def func():
    # Read Terraform code from local original.tf file
    try:
        with open("original.tf", "r") as f:
            tf = f.read()
    except FileNotFoundError:
        return JSONResponse(
            status_code=404,
            content={"error": "original.tf file not found in the current directory. Please ensure the file exists."}
        )
    except Exception as e:
        return JSONResponse(
            status_code=500,
            content={"error": f"Error reading original.tf file: {str(e)}"}
        )
    
    # Read cloud logs
    try:
        with open("logs.json","r") as f:
            cloud_logs = json.load(f)
    except FileNotFoundError:
        return JSONResponse(
            status_code=404,
            content={"error": "logs.json file not found in the current directory. Please ensure the file exists."}
        )
    except Exception as e:
        return JSONResponse(
            status_code=500,
            content={"error": f"Error reading logs.json file: {str(e)}"}
        )
    
    # Remove all previous output files (.txt and .tf files except original.tf)
    for file in os.listdir("."):
        if (file.endswith(".txt") or file.endswith(".tf")) and file != "original.tf":
            try:
                os.remove(file)
            except Exception as e:
                print(f"Warning: Could not remove file {file}: {e}")
    
    # Create a copy as original_terraform.tf for processing
    with open("original_terraform.tf", "w") as f:
        f.write(tf)
    # 1. Get initial user query for task planning
    initial_user_query = """\n\n1. Generate a graph representing the Terraform resources and their permissions using mermaid.js code in a original_graph.txt. The terraform code is present in original_terraform.tf
2. Analyze the original_terraform.tf code and the original_graph.txt together for security vulnerabilities. Create a detailed vulnerability report. You MUST use the 'write_file_tool' to save this report to 'report.txt' AND you MUST also output the full content of this report in a fixed format. It should be a JSON object with the list of vulnerabilities where each one of them should be a dictionary of keys-values: 'severity'(1,2 or 3. 1 being less severe and 3 being highly severe), 'vulnerabilities'(In markdown with a heading and what's the vulnerability), 'recommendations', 'resource name', 'resource type' and  'potential impact'. the below is an example:
[
    {
      "severity": 3,
      "vulnerability": "# Security Group allowing SSH from anywhere\nAllowing SSH access from 0.0.0.0/0 is a critical security risk as it exposes the server to potential unauthorized access from any IP address.",
      "recommendations": "Restrict SSH access to specific IP addresses or ranges that are trusted. Consider using a VPN or bastion host for secure access.",
      "resource_name": "new_sg",
      "resource_type": "aws_security_group",
      "potential_impact": "Unauthorized access to EC2 instances leading to data breaches or system compromise."
    },
    {
      ...
    }
]
3. Using the original_terraform.tf code, the vulnerability report in report.txt and the cloud trail logs in logs.json's content together, create a new version of the Terraform code that remediates the identified vulnerabilities. Only modify the vulnerable parts; keep all other parts of the code identical. The output of this step MUST be the complete new remediated Terraform code in changed_terraform.tf file.
4. Based on the newly created remediated Terraform code in changed_terraform.tf, generate a new graph representing the Terraform resources and their permissions using mermaid.js code in changed_graph.txt.

Ensure each task is distinct and focuses on clear objective. The output of one task might be crucial for the next.
NEVER EVER HALLUCINATE OR GIVE OUT ANY WRONG ANSWERS. UTILIZE ALL TOOLS WHEN NECESSARY."""#input("Please enter your high-level goal for the AI assistant: ")
    # Instantiate the Task Planner Agent
    task_planner = TaskPlannerAgent()

    # Generate tasks based on the user query
    tasks = task_planner.generate_tasks(initial_user_query)

    if not tasks or not isinstance(tasks, list) or not tasks[0].get("id") or "error" in tasks[0]["id"].lower():
        print("\nFailed to generate a valid task list from the Task Planner Agent.")
        if tasks and isinstance(tasks, list) and tasks[0].get("description"):
             print(f"Planner output: {tasks[0]['description']}") # Print error/fallback description
        print(f"Using a fallback task based on the original query: {initial_user_query}")
        tasks = [
            {
                "description": f"Attempt to address original query directly: {initial_user_query}",
                "id": "fallback_direct_query_task"
            }
        ]
    else:
        print("\n--- Generated Task List ---")
        for i, task_item in enumerate(tasks):
            print(f"{i+1}. ID: {task_item['id']}, Description: {task_item['description']}")
        print("--- End of Generated Task List ---")


    agent_init_kwargs = {"documentation_path": "./index"}


    # 3. Initialize loop-persistent context
    shared_memory_block = f"User role: Senior DevOps Engineer. Overall User Goal: {initial_user_query}"
    current_chat_history = []

    print("\n--- Starting Multi-Step Agent Execution ---")

    # 4. Loop through tasks generated by TaskPlannerAgent
    for i, task_info in enumerate(tasks):
        task_description = task_info["description"]
        task_id = task_info["id"]
        print(f"\n--- Iteration {i+1}/{len(tasks)} (Task ID: {task_id}) ---")
        print(f"Executing Task: {task_description}")

        agent = CloudAIAgent(**agent_init_kwargs)

        current_logs_snippet = f"Simulated logs for task {task_id}: System health nominal. No critical errors reported prior to this task."
        current_graph_data = f"{{'services': {{'service_Y': {{'status': 'unknown_at_start_of_task_{task_id}'}}}}}}"
        invocation_memory_block = f"{shared_memory_block} Current step: {i+1}/{len(tasks)}. Focus: {task_description[:80]}..."

        print(f"Invoking CloudAIAgent with memory hint: '{invocation_memory_block[:100]}...'")

        response = agent.invoke(
            task=task_description,
            cloud_logs=current_logs_snippet,
            graph_code=current_graph_data,
            memory_block=invocation_memory_block,
            chat_history=current_chat_history
        )

        agent_output = response.get('output', f'No output field in response. Full response: {response}')
        print(f"\n--- CloudAIAgent Output (Iteration {i+1}) ---")
        print(agent_output)

        current_chat_history.append({"role": "user", "content": task_description})
        current_chat_history.append({"role": "ai", "content": agent_output})

    print("\n--- All tasks processed ---")
    
    # open report.txt, original_graph.txt, changed_graph.txt, changed_terraform.tf

    with open("report.txt", "r") as f:
        report = f.read()
    with open("original_graph.txt", "r") as f:
        original_graph = f.read()
    with open("changed_graph.txt", "r") as f:
        changed_graph = f.read()
    with open("changed_terraform.tf", "r") as f:
        changed_terraform = f.read()

    return JSONResponse(content={
        "report": report,
        "original_graph": original_graph,
        "changed_graph": changed_graph,
        "changed_terraform": changed_terraform,
        "original_terraform": tf,
    })