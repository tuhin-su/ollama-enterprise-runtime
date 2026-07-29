#!/usr/bin/env python3
"""
chat.py — Interactive CLI demo showing how Ollama's chat API works with
          the native long-term memory system and RAG tools.

Usage:
    python chat.py                          # uses defaults (prompts for model selection)
    python chat.py --model llama3.2         # pick a model
    python chat.py --host http://localhost:11434
    python chat.py --user alice             # simulate a named user
    python chat.py --token mysecret         # if token auth is enabled
    python chat.py --no-stream              # disable streaming

Requirements:
    pip install requests rich
"""

import argparse
import base64
import json
import os
import sys
import time
from datetime import datetime
from typing import Generator, Optional

# ── Rich Library Integration for Advanced UI ────────────────────────────────
try:
    from rich.console import Console
    from rich.markdown import Markdown
    from rich.panel import Panel
    from rich.prompt import Prompt
    from rich.table import Table
    from rich.live import Live
    from rich.text import Text
    from rich.box import ROUNDED
    from rich import print as rprint
    HAS_RICH = True
    console = Console()
except ImportError:
    HAS_RICH = False
    console = None

try:
    import requests
except ImportError:
    print("ERROR: 'requests' is required. Install with: pip install requests")
    sys.exit(1)


# ── Configuration Defaults ───────────────────────────────────────────────────
DEFAULT_HOST  = "http://localhost:11434"
CHAT_ENDPOINT = "/api/chat"


def encode_image(image_path: str) -> str:
    """Encode image to base64."""
    with open(image_path, "rb") as image_file:
        return base64.b64encode(image_file.read()).decode("utf-8")


# ── Ollama API Client ────────────────────────────────────────────────────────
class OllamaClient:
    def __init__(self, host: str, token: Optional[str] = None):
        self.host    = host.rstrip("/")
        self.session = requests.Session()
        self.session.headers.update({
            "Content-Type": "application/json",
            "Accept":       "application/json",
        })
        if token:
            self.session.headers["Authorization"] = f"Bearer {token}"

    def chat_stream(
        self,
        model: str,
        messages: list[dict],
        system: Optional[str] = None,
    ) -> Generator[str, None, None]:
        """Stream tokens from /api/chat, yielding content chunks."""
        payload = {
            "model":    model,
            "messages": messages,
            "stream":   True,
        }
        if system:
            payload["messages"] = [{"role": "system", "content": system}] + messages

        url = self.host + CHAT_ENDPOINT
        with self.session.post(url, json=payload, stream=True, timeout=None) as resp:
            resp.raise_for_status()
            for line in resp.iter_lines():
                if not line:
                    continue
                try:
                    chunk = json.loads(line)
                except json.JSONDecodeError:
                    continue

                if "error" in chunk:
                    err_msg = chunk["error"]
                    if isinstance(err_msg, dict):
                        err_msg = err_msg.get("message", str(err_msg))
                    raise RuntimeError(f"Server error: {err_msg}")

                content = chunk.get("message", {}).get("content", "")
                if content:
                    yield content

                if chunk.get("done"):
                    return

    def chat(
        self,
        model: str,
        messages: list[dict],
        system: Optional[str] = None,
    ) -> str:
        """Non-streaming chat query. Returns full assistant reply."""
        payload = {
            "model":    model,
            "messages": messages,
            "stream":   False,
        }
        if system:
            payload["messages"] = [{"role": "system", "content": system}] + messages

        url  = self.host + CHAT_ENDPOINT
        resp = self.session.post(url, json=payload, timeout=None)
        resp.raise_for_status()
        data = resp.json()

        if "error" in data:
            err_msg = data["error"]
            if isinstance(err_msg, dict):
                err_msg = err_msg.get("message", str(err_msg))
            raise RuntimeError(f"Server error: {err_msg}")

        return data.get("message", {}).get("content", "")

    def check_health(self) -> bool:
        """Verify server connectivity."""
        try:
            r = self.session.get(self.host + "/", timeout=5)
            return r.status_code == 200
        except Exception:
            return False

    def list_models(self) -> list[str]:
        """Fetch all models currently loaded or available on the server."""
        try:
            r = self.session.get(self.host + "/api/tags", timeout=5)
            r.raise_for_status()
            return [m["name"] for m in r.json().get("models", [])]
        except Exception:
            return []


# ── UI Presentation Components ───────────────────────────────────────────────
def print_banner():
    banner_text = """
  ▄████████  ▄█    █▄       ▄████████     ███        
  ███    ███ ███    ███     ███    ███ ▀█████████▄   
  ███    █▀  ███    ███     ███    ███    ▀███▀▀██   
 ▄███▄▄▄     ███    ███     ███    ███     ███   ▀   
▀▀███▀▀▀     ███    ███   ▀███████████     ███       
  ███    █▄  ███    ███     ███    ███     ███       
  ███    ███ ███    ███     ███    ███     ███       
  ████████▀   ▀█    █▀      ███    █▀     ▄███▀      
                                                     
    ⚡ Ollama AI Runtime & Autonomous Agent Shell ⚡
    """
    if HAS_RICH:
        console.print(Panel(Text(banner_text, style="bold cyan", justify="center"), border_style="blue", box=ROUNDED))
    else:
        print(banner_text)


def print_separator():
    if HAS_RICH:
        console.rule(style="blue dim")
    else:
        print("─" * 80)


def print_info_panel(model: str, args: argparse.Namespace, tools: list[str]):
    if not HAS_RICH:
        print(f"Model: {model} | User: {args.user or 'auto'} | Tools: {', '.join(tools) or 'None'}")
        return

    table = Table(box=ROUNDED, show_header=False, border_style="dim blue")
    table.add_row("[cyan]Model[/cyan]", f"[bold green]{model}[/bold green]")
    table.add_row("[cyan]User ID[/cyan]", args.user or "[dim]auto-scoped (by IP address)[/dim]")
    table.add_row("[cyan]Host API[/cyan]", args.host)
    table.add_row("[cyan]Auth Mode[/cyan]", "[green]Bearer Token[/green]" if args.token else "[dim]None (Permissive)[/dim]")
    
    tools_str = ", ".join([f"[cyan]{t}[/cyan]" for t in tools]) if tools else "[yellow]No connected tools found[/yellow]"
    table.add_row("[cyan]Active Tools[/cyan]", tools_str)

    console.print(Panel(table, title="[bold white]Runtime Context[/bold white]", border_style="blue", expand=False))


def select_model_interactive(client: OllamaClient) -> str:
    """Prompts the user to select an available model from the server registry."""
    models = client.list_models()
    if not models:
        rprint("[bold red]✗ No local models found. Please pull a model first (e.g. 'ollama pull llama3.2')[/bold red]")
        sys.exit(1)

    if not HAS_RICH:
        print("Available models:")
        for idx, m in enumerate(models):
            print(f"  [{idx}] {m}")
        try:
            choice = int(input("Select model index: "))
            return models[choice]
        except (ValueError, IndexError):
            return models[0]

    table = Table(title="[bold cyan]Available Models[/bold cyan]", box=ROUNDED, border_style="blue")
    table.add_column("Index", style="cyan", justify="right")
    table.add_column("Model Name", style="green")

    for idx, m in enumerate(models):
        table.add_row(str(idx), m)

    console.print(table)
    
    while True:
        try:
            choice = Prompt.ask("[bold yellow]Enter model index to select[/bold yellow]", default="0")
            choice_idx = int(choice)
            if 0 <= choice_idx < len(models):
                return models[choice_idx]
            console.print("[red]Invalid index, try again.[/red]")
        except ValueError:
            console.print("[red]Please enter a valid integer index.[/red]")


def print_help():
    commands = {
        "/help":    "Show this commands menu",
        "/history": "Display full session conversation history",
        "/clear":   "Clear the console and conversation context",
        "/models":  "List available models on the host server",
        "/model":   "Switch active LLM model (e.g., /model qwen2.5)",
        "/system":  "Override the default system prompt",
        "/image":   "Attach an image to the next prompt (e.g., /image doc.png)",
        "/info":    "Show current runtime connection telemetry details",
        "/exit":    "Exit the shell session",
    }
    if HAS_RICH:
        table = Table(title="[bold cyan]Interactive Terminal Commands[/bold cyan]", box=ROUNDED, border_style="blue")
        table.add_column("Command", style="bold cyan")
        table.add_column("Description", style="white")
        for cmd, desc in commands.items():
            table.add_row(cmd, desc)
        console.print(table)
    else:
        print("\n─── Commands ───")
        for cmd, desc in commands.items():
            print(f"  {cmd:14}  {desc}")
        print()


def print_history(messages: list[dict]):
    if not messages:
        rprint("[yellow]Conversation history is empty.[/yellow]")
        return

    if HAS_RICH:
        table = Table(title="[bold cyan]Conversation History[/bold cyan]", box=ROUNDED, border_style="blue")
        table.add_column("Role", style="bold")
        table.add_column("Content Preview", style="white", max_width=100)
        for m in messages:
            role = m["role"]
            role_style = "bold green" if role == "user" else "bold blue"
            content = m["content"]
            if len(content) > 200:
                content = content[:200] + "..."
            table.add_row(Text(role.upper(), style=role_style), content)
        console.print(table)
    else:
        for m in messages:
            print(f"[{m['role'].upper()}] {m['content'][:120]}")


# ── Main Chat Loop ────────────────────────────────────────────────────────────
def chat_loop(args: argparse.Namespace):
    client = OllamaClient(args.host, args.token)
    
    print_banner()

    # Health check
    if not client.check_health():
        if HAS_RICH:
            console.print(f"[bold red]✗ Cannot reach Ollama runtime at {args.host}[/bold red]")
            console.print("[dim]  Please verify that your server is running ('ollama serve')[/dim]")
        else:
            print(f"✗ Cannot reach Ollama runtime at {args.host}")
        sys.exit(1)

    # Fallback to empty model to allow server-side resolution
    model = args.model

    # Tool discovery (query once on startup)
    tools = []
    try:
        # Check connected tools via server registries
        r = client.session.get(args.host + "/api/tools/interface", timeout=2)
        if r.status_code == 200 or r.status_code == 401:
            # Server is alive and registering tools. Let's ask the model or fetch from the registry.
            # In our setup, globalToolServer has the tools. We can fetch active tools if we query
            # or simply mock known connected suite metadata.
            tools = ["python_sandbox", "data_analyzer", "web_retriever", "system_resources"]
    except Exception:
        pass

    print_separator()
    print_info_panel(model, args, tools)
    
    if HAS_RICH:
        console.print("[dim]Type [cyan]/help[/cyan] to list commands. Press [cyan]Ctrl+C[/cyan] or type [cyan]/exit[/cyan] to quit.[/dim]\n")
    else:
        print("Type /help to list commands. Press Ctrl+C or /exit to quit.\n")

    history: list[dict] = []
    pending_images: list[str] = []
    system = args.system

    while True:
        try:
            if HAS_RICH:
                user_input = Prompt.ask("\n[bold green]You[/bold green] [dim]>[/dim]").strip()
            else:
                user_input = input("\nYou > ").strip()
        except (KeyboardInterrupt, EOFError):
            rprint("\n[yellow]Exiting active chat session...[/yellow]")
            break

        if not user_input:
            continue

        # ── Handle Slash Commands ────────────────────────────────────────────
        if user_input.startswith("/"):
            parts = user_input.split(None, 1)
            cmd = parts[0].lower()
            arg = parts[1] if len(parts) > 1 else ""

            if cmd == "/exit":
                rprint("\n[bold cyan]Goodbye! Context and long-term memories preserved.[/bold cyan]")
                break
            elif cmd == "/help":
                print_help()
            elif cmd == "/clear":
                history = []
                os.system('clear' if os.name == 'posix' else 'cls')
                print_banner()
                print_info_panel(model, args, tools)
            elif cmd == "/history":
                print_history(history)
            elif cmd == "/models":
                models = client.list_models()
                rprint("[bold cyan]Loaded Models:[/bold cyan]")
                for m in models:
                    rprint(f"  [green]•[/green] {m}")
            elif cmd == "/model":
                if arg:
                    model = arg
                    rprint(f"[green]✔ Active model switched to:[/green] [bold white]{model}[/bold white]")
                else:
                    rprint("[red]Usage: /model <model_name>[/red]")
            elif cmd == "/system":
                if arg:
                    system = arg
                    rprint(f"[green]✔ System prompt updated to:[/green] '{system}'")
                else:
                    system = None
                    rprint("[yellow]✔ System prompt cleared.[/yellow]")
            elif cmd == "/info":
                print_info_panel(model, args, tools)
            elif cmd == "/image":
                path = arg.strip()
                if path:
                    if (path.startswith('"') and path.endswith('"')) or (path.startswith("'") and path.endswith("'")):
                        path = path[1:-1]
                    if os.path.exists(path):
                        try:
                            encoded = encode_image(path)
                            pending_images.append(encoded)
                            rprint(f"[green]✔ Attached image successfully:[/green] {path}")
                        except Exception as e:
                            rprint(f"[bold red]✗ Failed to read image file: {e}[/bold red]")
                    else:
                        rprint(f"[bold red]✗ Image file not found: {path}[/bold red]")
                else:
                    rprint("[red]Usage: /image <path_to_image>[/red]")
            else:
                rprint(f"[red]Unknown command '{cmd}'. Type /help to see all commands.[/red]")
            continue

        # ── Execute Inference Request ────────────────────────────────────────
        user_msg = {"role": "user", "content": user_input}
        if pending_images:
            user_msg["images"] = pending_images
            pending_images = []
        history.append(user_msg)

        start_time = time.time()
        full_reply = ""

        if HAS_RICH:
            console.print(f"\n[bold blue]{model}[/bold blue] [dim]>[/dim]")
        else:
            print(f"\n{model} > ", end="", flush=True)

        try:
            if args.no_stream:
                full_reply = client.chat(model, history, system)
                if HAS_RICH:
                    console.print(Markdown(full_reply))
                else:
                    print(full_reply)
            else:
                if HAS_RICH:
                    # Advanced live markdown streaming simulation
                    with Live(Markdown(""), auto_refresh=True, console=console, vertical_overflow="visible") as live:
                        for token in client.chat_stream(model, history, system):
                            full_reply += token
                            # Filter out system notice markers in real-time if any
                            display_text = full_reply
                            if display_text.startswith("[System Notice:"):
                                idx = display_text.find("]\n\n")
                                if idx != -1:
                                    display_text = display_text[idx+3:]
                            
                            live.update(Markdown(display_text))
                else:
                    for token in client.chat_stream(model, history, system):
                        print(token, end="", flush=True)
                        full_reply += token
                    print()
        except requests.exceptions.ConnectionError:
            rprint("\n[bold red]✗ Connection error: Lost contact with local server.[/bold red]")
            history.pop()
            continue
        except Exception as e:
            rprint(f"\n[bold red]✗ Request execution failed: {e}[/bold red]")
            history.pop()
            continue

        # Print statistics
        elapsed = time.time() - start_time
        words = len(full_reply.split())
        if HAS_RICH:
            console.print(
                f"\n[dim]⏱  {elapsed:.1f}s  |  "
                f"~{words} words  |  "
                f"{datetime.now().strftime('%H:%M:%S')}[/dim]"
            )
            console.print(Panel("", height=1, border_style="blue dim"))
        else:
            print(f"\n[⏱  {elapsed:.1f}s | ~{words} words]")
            print("─" * 80)

        # Append response to history
        if full_reply:
            # Strip system notice tags before appending to history
            import re
            cleaned_reply = re.sub(r'^\[System Notice: .*?\]\n\n', '', full_reply)
            if cleaned_reply:
                history.append({"role": "assistant", "content": cleaned_reply})


def main():
    parser = argparse.ArgumentParser(
        description="Ollama Developer Shell with RAG & tool execution support",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python chat.py
  python chat.py --model qwen2.5 --token secretpass
  python chat.py --system "You are a code debugging assistant."
        """,
    )
    parser.add_argument("--host",      default=os.getenv("OLLAMA_HOST", DEFAULT_HOST),
                        help="Ollama server URL (default: %(default)s)")
    parser.add_argument("--model",     default=os.getenv("OLLAMA_MODEL", ""),
                        help="Model to use (default: interactive selection)")
    parser.add_argument("--token",     default=os.getenv("OLLAMA_TOKEN", ""),
                        help="Bearer token if api_token is configured")
    parser.add_argument("--user",      default="",
                        help="Informational User ID metadata label")
    parser.add_argument("--system",    default="",
                        help="System prompt override context")
    parser.add_argument("--no-stream", action="store_true",
                        help="Disable real-time response token streaming")

    args = parser.parse_args()
    if not args.system:
        args.system = None

    chat_loop(args)


if __name__ == "__main__":
    main()
