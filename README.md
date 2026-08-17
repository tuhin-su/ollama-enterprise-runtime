<p align="center">
  <a href="https://loom.com">
    <img src="https://github.com/loom/loom/assets/3325447/0d0b44e2-8f4a-4e99-9b52-a5c1c741c8f7" alt="loom" width="200"/>
  </a>
</p>

# Loom

Start building with open models.

## Loom vs This Fork (Agentic Loom)

While the original Loom is a fantastic tool for running LLMs locally, it primarily acts as a dumb execution engine. **This fork transforms Loom into a smart, autonomous, multi-model orchestrator.** 

Key differences in this fork:
- **Agentic Model Chaining:** A primary general-purpose model can dynamically delegate complex subtasks (vision, math, coding) to specialized models running locally.
- **Dynamic Routing & Model Awareness:** Modelfiles support a native `DESCRIPTION` keyword. When models interact, the backend injects descriptions of all installed models into their context so they "know" what other specialized models are available to them.
- **Anthropic Proxy Translation:** When connected to clients via the Anthropic (`/v1/messages`) compatibility layer, if an explicit model isn't requested (or "claude" is requested), the backend dynamically resolves the best local vision-capable model automatically.
- **Automatic Tool Injection:** Core system management tools (like `chain_request` and `system_management`) are natively and transparently injected, allowing models to query system health, reboot themselves, or route subtasks without the client application needing to write any orchestration logic.

See [docs/architecture.md](docs/architecture.md) for more details.

## Install

### Linux & macOS

```bash
curl -fsSL https://raw.githubusercontent.com/tuhin-su/loom-master/main/install.sh | sh
```

Auto-detects: `linux` / `darwin` × `amd64` / `arm64` / `arm`

### Windows

```powershell
irm https://raw.githubusercontent.com/tuhin-su/loom-master/main/install.ps1 | iex
```

Auto-detects: `amd64` / `arm64`. Optionally registers a Windows service.

### Manual Download

Download any binary from the [Releases page](https://github.com/tuhin-su/loom-master/releases).
Each asset includes a `.sha256` checksum file.

See [docs/installation.md](docs/installation.md) for full instructions, pinning a version, updating, and uninstalling.

### Docker

The official [Loom Docker image](https://hub.docker.com/r/loom/loom) `loom/loom` is available on Docker Hub.

### Libraries

- [loom-python](https://github.com/loom/loom-python)
- [loom-js](https://github.com/loom/loom-js)
## Get started

```
loom
```

You'll be prompted to run a model or connect Loom to your existing agents or applications such as `Claude Code`, `OpenClaw`, `OpenCode` , `Codex`, `Copilot`,  and more.

### Coding

To launch a specific integration:

```
loom launch claude
```

Supported integrations include [Claude Code](https://docs.loom.com/integrations/claude-code), [Codex](https://docs.loom.com/integrations/codex), [Copilot CLI](https://docs.loom.com/integrations/copilot-cli), [Droid](https://docs.loom.com/integrations/droid), and [OpenCode](https://docs.loom.com/integrations/opencode).

### AI assistant

Use [OpenClaw](https://docs.loom.com/integrations/openclaw) to turn Loom into a personal AI assistant across WhatsApp, Telegram, Slack, Discord, and more:

```
loom launch openclaw
```

### Chat with a model

Run and chat with [Gemma 4](https://loom.com/library/gemma4):

```
loom run gemma4
```

See [loom.com/library](https://loom.com/library) for the full list.

See the [quickstart guide](https://docs.loom.com/quickstart) for more details.

### Importing GGUF Models

You can import any local GGUF file directly into Loom using the `loom import` command:

```shell
# Import a model (name is auto-derived)
loom import /path/to/model.gguf

# Specify a custom name
loom import /path/to/model.gguf --name my-model

# Import a Vision-Language (VL) model with its multimodal projector
loom import /path/to/model.gguf --mmproj /path/to/mmproj.gguf --name my-vl-model
```

### Exporting GGUF Models

You can export GGUF files back out of Loom's local storage:

```shell
# Export a model
loom export my-model model.gguf

# Export a VL model (projector is auto-exported as well)
loom export my-vl-model model.gguf
```

## Agentic Orchestration & Capabilities

Loom supports **Model Chaining** and **Agentic Orchestration** locally:
- **Automatic Tool Injection:** The `chain_request` tool is automatically injected into models. 
- **Dynamic Routing:** Loom can parse the `DESCRIPTION` keyword inside your `Modelfile` to give models a dynamic awareness of other models running locally on your hardware. 
- **Subtask Delegation:** Generic models can intelligently offload specialized workloads (such as image analysis, coding, or math) to specific models based on their descriptions.

See [docs/architecture.md](docs/architecture.md) for architecture details, and the [Tool & Client Developer Guide](docs/tool-developer-guide.md) for information on building apps and natively registering custom tools!

## REST API

Loom has a REST API for running and managing models.

```
curl http://localhost:11434/api/chat -d '{
  "model": "gemma4",
  "messages": [{
    "role": "user",
    "content": "Why is the sky blue?"
  }],
  "stream": false
}'
```

See the [API documentation](https://docs.loom.com/api) for all endpoints.

### Python

```
pip install loom
```

```python
from loom import chat

response = chat(model='gemma4', messages=[
  {
    'role': 'user',
    'content': 'Why is the sky blue?',
  },
])
print(response.message.content)
```

### JavaScript

```
npm i loom
```

```javascript
import loom from "loom";

const response = await loom.chat({
  model: "gemma4",
  messages: [{ role: "user", content: "Why is the sky blue?" }],
});
console.log(response.message.content);
```

## Supported backends

- [llama.cpp](https://github.com/ggml-org/llama.cpp) project founded by Georgi Gerganov.

## Documentation

- [CLI reference](https://docs.loom.com/cli)
- [REST API reference](https://docs.loom.com/api)
- [Installation guide](docs/installation.md) — all platforms, update, uninstall
- [Server configuration](docs/server-config.md) — token auth, memory system
- [Long-term memory](docs/memory.md) — native persistent memory across sessions
- [Importing models](https://docs.loom.com/import)
- [Modelfile reference](https://docs.loom.com/modelfile)
- [Building from source](https://github.com/loom/loom/blob/main/docs/development.md)

## Community Integrations

> Want to add your project? Open a pull request.

### Chat Interfaces

#### Web

- [Open WebUI](https://github.com/open-webui/open-webui) - Extensible, self-hosted AI interface
- [Onyx](https://github.com/onyx-dot-app/onyx) - Connected AI workspace
- [LibreChat](https://github.com/danny-avila/LibreChat) - Enhanced ChatGPT clone with multi-provider support
- [Lobe Chat](https://github.com/lobehub/lobe-chat) - Modern chat framework with plugin ecosystem ([docs](https://lobehub.com/docs/self-hosting/examples/loom))
- [NextChat](https://github.com/ChatGPTNextWeb/ChatGPT-Next-Web) - Cross-platform ChatGPT UI ([docs](https://docs.nextchat.dev/models/loom))
- [Perplexica](https://github.com/ItzCrazyKns/Perplexica) - AI-powered search engine, open-source Perplexity alternative
- [big-AGI](https://github.com/enricoros/big-AGI) - AI suite for professionals
- [Lollms WebUI](https://github.com/ParisNeo/lollms-webui) - Multi-model web interface
- [ChatLoom](https://github.com/sugarforever/chat-loom) - Chatbot with knowledge bases
- [Bionic GPT](https://github.com/bionic-gpt/bionic-gpt) - On-premise AI platform
- [Chatbot UI](https://github.com/ivanfioravanti/chatbot-loom) - ChatGPT-style web interface
- [Hloom](https://github.com/fmaclen/hloom) - Minimal web interface
- [Chatbox](https://github.com/Bin-Huang/Chatbox) - Desktop and web AI client
- [chat](https://github.com/swuecho/chat) - Chat web app for teams
- [Loom RAG Chatbot](https://github.com/datvodinh/rag-chatbot.git) - Chat with multiple PDFs using RAG
- [Tkinter-based client](https://github.com/chyok/loom-gui) - Python desktop client

#### Desktop

- [Dify.AI](https://github.com/langgenius/dify) - LLM app development platform
- [AnythingLLM](https://github.com/Mintplex-Labs/anything-llm) - All-in-one AI app for Mac, Windows, and Linux
- [Maid](https://github.com/Mobile-Artificial-Intelligence/maid) - Cross-platform mobile and desktop client
- [Witsy](https://github.com/nbonamy/witsy) - AI desktop app for Mac, Windows, and Linux
- [Cherry Studio](https://github.com/kangfenmao/cherry-studio) - Multi-provider desktop client
- [Loom App](https://github.com/JHubi1/loom-app) - Multi-platform client for desktop and mobile
- [PyGPT](https://github.com/szczyglis-dev/py-gpt) - AI desktop assistant for Linux, Windows, and Mac
- [Alpaca](https://github.com/Jeffser/Alpaca) - GTK4 client for Linux and macOS
- [SwiftChat](https://github.com/aws-samples/swift-chat) - Cross-platform including iOS, Android, and Apple Vision Pro
- [Enchanted](https://github.com/AugustDev/enchanted) - Native macOS and iOS client
- [RWKV-Runner](https://github.com/josStorer/RWKV-Runner) - Multi-model desktop runner
- [Loom Grid Search](https://github.com/dezoito/loom-grid-search) - Evaluate and compare models
- [macai](https://github.com/Renset/macai) - macOS client for Loom and ChatGPT
- [AI Studio](https://github.com/MindWorkAI/AI-Studio) - Multi-provider desktop IDE
- [Reins](https://github.com/ibrahimcetin/reins) - Parameter tuning and reasoning model support
- [ConfiChat](https://github.com/1runeberg/confichat) - Privacy-focused with optional encryption
- [LLocal.in](https://github.com/kartikm7/llocal) - Electron desktop client
- [MindMac](https://mindmac.app) - AI chat client for Mac
- [Msty](https://msty.app) - Multi-model desktop client
- [BoltAI for Mac](https://boltai.com) - AI chat client for Mac
- [IntelliBar](https://intellibar.app/) - AI-powered assistant for macOS
- [Kerlig AI](https://www.kerlig.com/) - AI writing assistant for macOS
- [Hillnote](https://hillnote.com) - Markdown-first AI workspace
- [Perfect Memory AI](https://www.perfectmemory.ai/) - Productivity AI personalized by screen and meeting history

#### Mobile

- [Loom Android Chat](https://github.com/sunshine0523/LoomServer) - One-click Loom on Android

> SwiftChat, Enchanted, Maid, Loom App, Reins, and ConfiChat listed above also support mobile platforms.

### Code Editors & Development

- [Cline](https://github.com/cline/cline) - VS Code extension for multi-file/whole-repo coding
- [Continue](https://github.com/continuedev/continue) - Open-source AI code assistant for any IDE
- [Void](https://github.com/voideditor/void) - Open source AI code editor, Cursor alternative
- [Copilot for Obsidian](https://github.com/logancyang/obsidian-copilot) - AI assistant for Obsidian
- [twinny](https://github.com/rjmacarthy/twinny) - Copilot and Copilot chat alternative
- [gptel Emacs client](https://github.com/karthink/gptel) - LLM client for Emacs
- [Loom Copilot](https://github.com/bernardo-bruning/loom-copilot) - Use Loom as GitHub Copilot
- [Obsidian Local GPT](https://github.com/pfrankov/obsidian-local-gpt) - Local AI for Obsidian
- [Ellama Emacs client](https://github.com/s-kostyaev/ellama) - LLM tool for Emacs
- [orbiton](https://github.com/xyproto/orbiton) - Config-free text editor with Loom tab completion
- [AI ST Completion](https://github.com/yaroslavyaroslav/OpenAI-sublime-text) - Sublime Text 4 AI assistant
- [VT Code](https://github.com/vinhnx/vtcode) - Rust-based terminal coding agent with Tree-sitter
- [QodeAssist](https://github.com/Palm1r/QodeAssist) - AI coding assistant for Qt Creator
- [AI Toolkit for VS Code](https://aka.ms/ai-tooklit/loom-docs) - Microsoft-official VS Code extension
- [Open Interpreter](https://docs.openinterpreter.com/language-model-setup/local-models/loom) - Natural language interface for computers

### Libraries & SDKs

- [LiteLLM](https://github.com/BerriAI/litellm) - Unified API for 100+ LLM providers
- [Semantic Kernel](https://github.com/microsoft/semantic-kernel/tree/main/python/semantic_kernel/connectors/ai/loom) - Microsoft AI orchestration SDK
- [LangChain4j](https://github.com/langchain4j/langchain4j) - Java LangChain ([example](https://github.com/langchain4j/langchain4j-examples/tree/main/loom-examples/src/main/java))
- [LangChainGo](https://github.com/tmc/langchaingo/) - Go LangChain ([example](https://github.com/tmc/langchaingo/tree/main/examples/loom-completion-example))
- [Spring AI](https://github.com/spring-projects/spring-ai) - Spring framework AI support ([docs](https://docs.spring.io/spring-ai/reference/api/chat/loom-chat.html))
- [LangChain](https://python.langchain.com/docs/integrations/chat/loom/) and [LangChain.js](https://js.langchain.com/docs/integrations/chat/loom/) with [example](https://js.langchain.com/docs/tutorials/local_rag/)
- [Loom for Ruby](https://github.com/crmne/ruby_llm) - Ruby LLM library
- [any-llm](https://github.com/mozilla-ai/any-llm) - Unified LLM interface by Mozilla
- [LoomSharp for .NET](https://github.com/awaescher/LoomSharp) - .NET SDK
- [LangChainRust](https://github.com/Abraxas-365/langchain-rust) - Rust LangChain ([example](https://github.com/Abraxas-365/langchain-rust/blob/main/examples/llm_loom.rs))
- [Agents-Flex for Java](https://github.com/agents-flex/agents-flex) - Java agent framework ([example](https://github.com/agents-flex/agents-flex/tree/main/agents-flex-llm/agents-flex-llm-loom/src/test/java/com/agentsflex/llm/loom))
- [Elixir LangChain](https://github.com/brainlid/langchain) - Elixir LangChain
- [Loom-rs for Rust](https://github.com/pepperoni21/loom-rs) - Rust SDK
- [LangChain for .NET](https://github.com/tryAGI/LangChain) - .NET LangChain ([example](https://github.com/tryAGI/LangChain/blob/main/examples/LangChain.Samples.OpenAI/Program.cs))
- [chromem-go](https://github.com/philippgille/chromem-go) - Go vector database with Loom embeddings ([example](https://github.com/philippgille/chromem-go/tree/v0.5.0/examples/rag-wikipedia-loom))
- [LangChainDart](https://github.com/davidmigloz/langchain_dart) - Dart LangChain
- [LlmTornado](https://github.com/lofcz/llmtornado) - Unified C# interface for multiple inference APIs
- [Loom4j for Java](https://github.com/loom4j/loom4j) - Java SDK
- [Loom for Laravel](https://github.com/cloudstudio/loom-laravel) - Laravel integration
- [Loom for Swift](https://github.com/mattt/loom-swift) - Swift SDK
- [LlamaIndex](https://docs.llamaindex.ai/en/stable/examples/llm/loom/) and [LlamaIndexTS](https://ts.llamaindex.ai/modules/llms/available_llms/loom) - Data framework for LLM apps
- [Haystack](https://github.com/deepset-ai/haystack-integrations/blob/main/integrations/loom.md) - AI pipeline framework
- [Firebase Genkit](https://firebase.google.com/docs/genkit/plugins/loom) - Google AI framework
- [Loom-hpp for C++](https://github.com/jmont-dev/loom-hpp) - C++ SDK
- [PromptingTools.jl](https://github.com/svilupp/PromptingTools.jl) - Julia LLM toolkit ([example](https://svilupp.github.io/PromptingTools.jl/dev/examples/working_with_loom))
- [Loom for R - rloom](https://github.com/JBGruber/rloom) - R SDK
- [Portkey](https://portkey.ai/docs/welcome/integration-guides/loom) - AI gateway
- [Testcontainers](https://testcontainers.com/modules/loom/) - Container-based testing
- [LLPhant](https://github.com/theodo-group/LLPhant?tab=readme-ov-file#loom) - PHP AI framework

### Frameworks & Agents

- [AutoGPT](https://github.com/Significant-Gravitas/AutoGPT/blob/master/docs/content/platform/loom.md) - Autonomous AI agent platform
- [crewAI](https://github.com/crewAIInc/crewAI) - Multi-agent orchestration framework
- [Strands Agents](https://github.com/strands-agents/sdk-python) - Model-driven agent building by AWS
- [Cheshire Cat](https://github.com/cheshire-cat-ai/core) - AI assistant framework
- [any-agent](https://github.com/mozilla-ai/any-agent) - Unified agent framework interface by Mozilla
- [Stakpak](https://github.com/stakpak/agent) - Open source DevOps agent
- [Hexabot](https://github.com/hexastack/hexabot) - Conversational AI builder
- [Neuro SAN](https://github.com/cognizant-ai-lab/neuro-san-studio) - Multi-agent orchestration ([docs](https://github.com/cognizant-ai-lab/neuro-san-studio/blob/main/docs/user_guide.md#loom))

### RAG & Knowledge Bases

- [RAGFlow](https://github.com/infiniflow/ragflow) - RAG engine based on deep document understanding
- [R2R](https://github.com/SciPhi-AI/R2R) - Open-source RAG engine
- [MaxKB](https://github.com/1Panel-dev/MaxKB/) - Ready-to-use RAG chatbot
- [Minima](https://github.com/dmayboroda/minima) - On-premises or fully local RAG
- [Chipper](https://github.com/TilmanGriesel/chipper) - AI interface with Haystack RAG
- [ARGO](https://github.com/xark-argo/argo) - RAG and deep research on Mac/Windows/Linux
- [Archyve](https://github.com/nickthecook/archyve) - RAG-enabling document library
- [Casibase](https://casibase.org) - AI knowledge base with RAG and SSO
- [BrainSoup](https://www.nurgo-software.com/products/brainsoup) - Native client with RAG and multi-agent automation

### Bots & Messaging

- [LangBot](https://github.com/RockChinQ/LangBot) - Multi-platform messaging bots with agents and RAG
- [AstrBot](https://github.com/Soulter/AstrBot/) - Multi-platform chatbot with RAG and plugins
- [Discord-Loom Chat Bot](https://github.com/kevinthedang/discord-loom) - TypeScript Discord bot
- [Loom Telegram Bot](https://github.com/ruecat/loom-telegram) - Telegram bot
- [LLM Telegram Bot](https://github.com/innightwolfsleep/llm_telegram_bot) - Telegram bot for roleplay

### Terminal & CLI

- [aichat](https://github.com/sigoden/aichat) - All-in-one LLM CLI with Shell Assistant, RAG, and AI tools
- [oterm](https://github.com/ggozad/oterm) - Terminal client for Loom
- [gloom](https://github.com/sammcj/gloom) - Go-based model manager for Loom
- [tlm](https://github.com/yusufcanb/tlm) - Local shell copilot
- [tenere](https://github.com/pythops/tenere) - TUI for LLMs
- [ParLlama](https://github.com/paulrobello/parllama) - TUI for Loom
- [llm-loom](https://github.com/taketwo/llm-loom) - Plugin for [Datasette's LLM CLI](https://llm.datasette.io/en/stable/)
- [ShellOracle](https://github.com/djcopley/ShellOracle) - Shell command suggestions
- [LLM-X](https://github.com/mrdjohnson/llm-x) - Progressive web app for LLMs
- [cmdh](https://github.com/pgibler/cmdh) - Natural language to shell commands
- [VT](https://github.com/vinhnx/vt.ai) - Minimal multimodal AI chat app

### Productivity & Apps

- [AppFlowy](https://github.com/AppFlowy-IO/AppFlowy) - AI collaborative workspace, self-hostable Notion alternative
- [Screenpipe](https://github.com/mediar-ai/screenpipe) - 24/7 screen and mic recording with AI-powered search
- [Vibe](https://github.com/thewh1teagle/vibe) - Transcribe and analyze meetings
- [Page Assist](https://github.com/n4ze3m/page-assist) - Chrome extension for AI-powered browsing
- [NativeMind](https://github.com/NativeMindBrowser/NativeMindExtension) - Private, on-device browser AI assistant
- [Loom Fortress](https://github.com/ParisNeo/loom_proxy_server) - Security proxy for Loom
- [1Panel](https://github.com/1Panel-dev/1Panel/) - Web-based Linux server management
- [Writeopia](https://github.com/Writeopia/Writeopia) - Text editor with Loom integration
- [QA-Pilot](https://github.com/reid41/QA-Pilot) - GitHub code repository understanding
- [Raycast extension](https://github.com/MassimilianoPasquini97/raycast_loom) - Loom in Raycast
- [Painting Droid](https://github.com/mateuszmigas/painting-droid) - Painting app with AI integrations
- [Serene Pub](https://github.com/doolijb/serene-pub) - AI roleplaying app
- [Mayan EDMS](https://gitlab.com/mayan-edms/mayan-edms) - Document management with Loom workflows
- [TagSpaces](https://www.tagspaces.org) - File management with [AI tagging](https://docs.tagspaces.org/ai/)

### Observability & Monitoring

- [Opik](https://www.comet.com/docs/opik/cookbook/loom) - Debug, evaluate, and monitor LLM applications
- [OpenLIT](https://github.com/openlit/openlit) - OpenTelemetry-native monitoring for Loom and GPUs
- [Lunary](https://lunary.ai/docs/integrations/loom) - LLM observability with analytics and PII masking
- [Langfuse](https://langfuse.com/docs/integrations/loom) - Open source LLM observability
- [HoneyHive](https://docs.honeyhive.ai/integrations/loom) - AI observability and evaluation for agents
- [MLflow Tracing](https://mlflow.org/docs/latest/llms/tracing/index.html#automatic-tracing) - Open source LLM observability

### Database & Embeddings

- [pgai](https://github.com/timescale/pgai) - PostgreSQL as a vector database ([guide](https://github.com/timescale/pgai/blob/main/docs/vectorizer-quick-start.md))
- [MindsDB](https://github.com/mindsdb/mindsdb/blob/staging/mindsdb/integrations/handlers/loom_handler/README.md) - Connect Loom with 200+ data platforms
- [chromem-go](https://github.com/philippgille/chromem-go/blob/v0.5.0/embed_loom.go) - Embeddable vector database for Go ([example](https://github.com/philippgille/chromem-go/tree/v0.5.0/examples/rag-wikipedia-loom))
- [Kangaroo](https://github.com/dbkangaroo/kangaroo) - AI-powered SQL client

### Infrastructure & Deployment

#### Cloud

- [Google Cloud](https://cloud.google.com/run/docs/tutorials/gpu-gemma2-with-loom)
- [Fly.io](https://fly.io/docs/python/do-more/add-loom/)
- [Koyeb](https://www.koyeb.com/deploy/loom)
- [Harbor](https://github.com/av/harbor) - Containerized LLM toolkit with Loom as default backend

#### Package Managers

- [Pacman](https://archlinux.org/packages/extra/x86_64/loom/)
- [Homebrew](https://formulae.brew.sh/formula/loom)
- [Nix package](https://search.nixos.org/packages?show=loom&from=0&size=50&sort=relevance&type=packages&query=loom)
- [Helm Chart](https://artifacthub.io/packages/helm/loom-helm/loom)
- [Gentoo](https://github.com/gentoo/guru/tree/master/app-misc/loom)
- [Flox](https://flox.dev/blog/loom-part-one)
- [Guix channel](https://codeberg.org/tusharhero/loom-guix)
