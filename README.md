# CloudLoom

CloudLoom is a cloud security platform that automatically detects misconfigurations and threats across AWS infrastructure and uses AI to both explain and remediate them.
It monitors CloudTrail for changes (for example: someone makes an S3 bucket public or attaches an overly permissive IAM policy). The Go backend ingests events, sends context to an AI agent connected to a RAG pipeline (seeded with CIS Benchmarks and org policies), and the agent returns a verdict, a human-readable explanation, and a recommended remediation. If the change originated from a GitHub PR, CloudLoom can generate an AI-suggested PR to fix the misconfiguration. Users can preview the fix and apply it with one click; the backend executes the remediation securely using the AWS SDK.

## Quick links (repo layout)
- `backend/` — Go backend service (API handlers, CloudTrail ingestion, remediation executor, AWS integrations).
	- `backend/main.go` — service entry.
	- `backend/api/` — API routes and handlers (assume-role, cloudformation, infrastructure, etc.).
	- `backend/services/` — AWS service integrations (S3, STS, CloudTrail, CloudWatch, SQS, etc.).
	- `backend/cloudformation-templates/` — sample CloudFormation templates used for remediation or bootstrapping.
	- `backend/config/`, `backend/common/`, `backend/models/` — configuration, globals and data models.
- `frontend/` — Vite + React TypeScript frontend. Dashboard UI, resource graph, remediation workflows.
- `multi_role_agent/` — Python agent experiments, Terraform/IaC patch generation, tests and scripts.
- `docs/infra/` — infra-related docs and helper scripts.
- Root-level test scripts: `test_diagram_generation.sh`, `test_mermaid_handler.sh`, etc.

## High-level architecture & data flow

1. Observation: AWS CloudTrail records actions and configuration changes.
2. Ingest: CloudTrail events are routed to the backend (EventBridge / SQS or direct webhook).
3. Enrichment: The backend enriches the event with resource inventory and relationship graph (Steampipe or internal inventory).
4. RAG + AI: Enriched context is sent to a RAG pipeline grounded on CIS Benchmarks and internal policies.
5. Decision & Explain: The AI outputs a verdict (severity), an explanation, and one or more remediation options.
6. Traceback & PRs: If the change originated from source control, the system attempts to trace it to the originating GitHub PR and can generate a suggested fix PR.
7. Apply: User can accept a fix via the UI. The backend validates and applies the remediation (CloudFormation/Terraform change or direct AWS SDK call), logging all actions.
8. UI Update: Dashboard updates in real-time with new risk scores, remediation history, and attack-chain visualizations.

Diagram (text):
CloudTrail -> EventBridge/SQS -> `backend` handlers -> RAG AI (CIS index) -> remediation proposal -> GitHub/CloudFormation/Terraform/AWS SDK -> Dashboard

## Key concepts and components

Steampipe
- Optional: Steampipe can be used to query AWS and build an inventory/relationship graph using SQL-like queries. Integrate to augment the resource graph and produce richer context for the AI.

CloudTrail & AWS
- CloudTrail is the single source of truth for changes. CloudLoom relies on CloudTrail events to detect when sensitive configuration changes occur (e.g. public S3 ACLs, policy attachments).

IAM, STS & secure access
- The backend uses short-lived credentials and STS assume-role patterns to act in target accounts safely. See `backend/api/assume-role/` for assume-role handler logic.
- Principle of least privilege must be applied to remediation roles. Audit and logging record every remediation action.

AI agent & RAG pipeline
- Retrieval-Augmented Generation (RAG) is used to ground LLM responses in CIS Benchmarks, cloud provider docs, and org policies.
- The agent returns:
	- a verdict and severity score,
	- a human-readable explanation of risk and exploitation path,
	- a recommended remediation (Terraform/CloudFormation diff, CLI steps, or SDK calls),
	- when applicable, a suggested Git patch/PR for IaC fixes.

Remediation flow
- The UI shows remediation proposals with diffs. A one-click action sends the selected remediation to the backend which:
	1. validates the change (dry-run or CI check),
	2. creates a PR (if source-controlled), or
	3. applies the change via CloudFormation/Terraform or the AWS SDK.

Multi-step attack chain visualization
- The resource graph connects resources, permissions, and identities to show how an attacker could chain access. Nodes are annotated with risk and AI explanations.

## Developer quickstart

Prerequisites
- Go 1.21+
- Node.js 18+ and npm or pnpm
- Python 3.10+ for `multi_role_agent` experiments (optional)
- Terraform / CloudFormation CLIs if you plan to apply templates
- AWS CLI configured with a profile that can create the required roles and resources
- (Optional) Vector DB / embedding provider + LLM endpoint for the RAG pipeline

Environment variables (example)

```
BACKEND_PORT=8080
AWS_PROFILE=default
AWS_REGION=us-east-1
MONGO_URI=mongodb://localhost:27017/cloudloom
AI_PROVIDER_ENDPOINT=https://your-llm-endpoint.example
AI_API_KEY=REPLACE_ME
GITHUB_TOKEN=ghp_...
```

Run backend (development)

```bash
cd backend
go build ./...
BACKEND_PORT=8080 AWS_PROFILE=default go run main.go
```

Run frontend (development)

```bash
cd frontend
npm install
npm run dev
# or with pnpm
pnpm install
pnpm run dev
```

Run the Python agent experiments (optional)

```bash
cd multi_role_agent
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python agent.py
```

Testing & linting
- Backend: `go test ./...`
- Frontend: `npm test` (if tests present) and `npm run build` to validate the app bundle
- Linting: use your preferred linters (`golangci-lint`, `eslint`, `typescript` checks)

Smoke tests
- The repo includes a few shell scripts for quick checks (see `test_diagram_generation.sh`, `test_mermaid_handler.sh`). Use them to validate small parts of the system locally.

## Deployment & infra
- Use `backend/cloudformation-templates/` for CloudFormation-based bootstrapping.
- For production, deploy the backend behind an API gateway and a load balancer, use IAM roles with least privilege, and store secrets in a secrets manager (AWS Secrets Manager / HashiCorp Vault).
- Configure CloudTrail to send events to EventBridge or SQS and ensure the backend can consume those events.

## Security guidance
- Don’t commit secrets or API keys. Use environment variables and secret stores.
- Log all remediation actions and provide an audit trail with time, actor, and justification.
- Default to human-in-the-loop for automated fixes. Allow an opt-in automatic remediation flow for mature environments.

## Troubleshooting
- CloudTrail events missing: verify CloudTrail, S3 delivery or EventBridge routing, and IAM permissions for the backend to read the events.
- Permission errors applying fixes: ensure the remediation role has the necessary permissions and STS is working.
- AI suggestions are low quality: improve the RAG corpus (add CIS docs and provider best-practices) and verify embeddings.

## Contributing
- Fork and use feature branches. Add tests and documentation for any new behavior.
- Include a clear PR description and security review for remediation changes.

## Files of immediate interest
- `backend/main.go` — backend entrypoint
- `backend/api/*/handlers.go` — API surface
- `backend/services/*` — AWS & remediation logic
- `backend/cloudformation-templates/` — remediation/bootstrapping templates
- `frontend/src/components/*` — UI components and visualization
- `multi_role_agent/agent.py` — agent experiments and IaC patching

## Roadmap ideas
- Harden retrieval indexing for CIS and org policies.
- Add dry-run and automatic rollback support for automated remediations.
- Add simulated attack chain unit/integration tests.

## License & acknowledgments
- Add a `LICENSE` file at repo root. Acknowledge third-party libraries in a `NOTICE`.

---

If you'd like, I can also add a `.env.example` or `README_SETUP.md` with environment variable templates and a small `scripts/` folder containing quickstart commands.


