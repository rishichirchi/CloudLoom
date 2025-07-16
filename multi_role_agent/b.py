from pathlib import Path
from docling.datamodel.base_models import InputFormat
from langchain.text_splitter import MarkdownTextSplitter # Import MarkdownTextSplitter
    # Required imports for create_markdown_faiss_index (expected to be in the global scope):
from langchain.docstore.document import Document
from langchain_huggingface import HuggingFaceEmbeddings
from langchain_community.vectorstores import Chroma
import os # Though not directly used in create_markdown_faiss_index, FAISS.save_local uses path operations.



def create_markdown_faiss_index(
    markdown_content: str,
    index_path: str = "faiss_markdown_index",
    embeddings_model_name: str = "intfloat/e5-large-v2"
):
    """
    Chunks a Markdown string, creates embeddings, builds a FAISS index,
    and saves it locally.
    Relies on Document, HuggingFaceEmbeddings, and FAISS being available from imports.
    """
    print(f"\n--- Creating FAISS index from Markdown content at {index_path} ---")

    # 1. Create a Document object from the markdown content
    # Ensure Document is imported: from langchain.docstore.document import Document
    doc = Document(page_content=markdown_content, metadata={"source": "local_markdown"})

    # 2. Split the Markdown document
    markdown_splitter = MarkdownTextSplitter(chunk_size=1500, chunk_overlap=350)
    split_docs = markdown_splitter.split_documents([doc])
    print(f"Markdown content split into {len(split_docs)} chunks.")
    if not split_docs:
        print("No chunks were created from the markdown content. Aborting index creation.")
        return

    # 3. Initialize embeddings
    # Ensure HuggingFaceEmbeddings is imported: from langchain_huggingface import HuggingFaceEmbeddings
    try:
        embeddings = HuggingFaceEmbeddings(model_name=embeddings_model_name,model_kwargs={"device":"cuda"})
    except Exception as e:
        print(f"Error initializing HuggingFaceEmbeddings: {e}")
        print("Please ensure sentence-transformers is installed and the model is accessible.")
        return

    # 4. Create FAISS vector store
    # Ensure FAISS is imported: from langchain_community.vectorstores import FAISS
    try:
        print("Creating FAISS vector store...")
        # os.makedirs(index_path)
        vectorstore = Chroma.from_documents(split_docs, embeddings, persist_directory=index_path)
        print("FAISS vector store created successfully.")
    except Exception as e:
        print(f"Error creating FAISS vector store: {e}")
        print("Ensure 'faiss-cpu' or 'faiss-gpu' is installed.")
        return

    # 5. Save the FAISS index locally
    try:
        print(f"FAISS index saved locally to folder: {index_path}")
    except Exception as e:
        print(f"Error saving FAISS index: {e}")

        
f = open("doc.md", "r")
sample_markdown = f.read()
f.close()
create_markdown_faiss_index(sample_markdown, index_path=r"E:\weird shit\Aventus\index")
