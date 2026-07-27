#!/usr/bin/env python3
"""
chat.py — Interactive CLI demo showing how Ollama's chat API works with
          the native long-term memory system.

Usage:
    python chat.py                          # uses defaults
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

# ── Optional rich for pretty output ──────────────────────────────────────────
try:
    from rich.console import Console
    from rich.markdown import Markdown
    from rich.panel import Panel
    from rich.prompt import Prompt
    from rich.table import Table
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


# ── Config ────────────────────────────────────────────────────────────────────
DEFAULT_HOST  = "http://localhost:11434"
DEFAULT_MODEL = "qwen2.5-vl-7b:latest"
CHAT_ENDPOINT = "/api/chat"
MEMORY_NOTE   = "(Memory context injected automatically by the server)"


def encode_image(image_path: str) -> str:
    """Encode image to base64."""
    with open(image_path, "rb") as image_file:
        return base64.b64encode(image_file.read()).decode("utf-8")


# ── Ollama client ─────────────────────────────────────────────────────────────
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
        """Stream tokens from /api/chat, yield content strings."""
        payload = {
            "model":    model,
            "messages": messages,
            "stream":   True,
        }
        if system:
            payload["messages"] = [{"role": "system", "content": system}] + messages

        url = self.host + CHAT_ENDPOINT
        with self.session.post(url, json=payload, stream=True, timeout=120) as resp:
            resp.raise_for_status()
            for line in resp.iter_lines():
                if not line:
                    continue
                try:
                    chunk = json.loads(line)
                except json.JSONDecodeError:
                    continue

                if "error" in chunk:
                    raise RuntimeError(f"Server error: {chunk['error']}")

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
        """Non-streaming /api/chat. Returns full assistant reply."""
        payload = {
            "model":    model,
            "messages": messages,
            "stream":   False,
        }
        if system:
            payload["messages"] = [{"role": "system", "content": system}] + messages

        url  = self.host + CHAT_ENDPOINT
        resp = self.session.post(url, json=payload, timeout=120)
        resp.raise_for_status()
        data = resp.json()

        if "error" in data:
            raise RuntimeError(f"Server error: {data['error']}")

        return data.get("message", {}).get("content", "")

    def check_health(self) -> bool:
        try:
            r = self.session.get(self.host + "/", timeout=5)
            return r.status_code == 200
        except Exception:
            return False

    def list_models(self) -> list[str]:
        try:
            r = self.session.get(self.host + "/api/tags", timeout=10)
            r.raise_for_status()
            return [m["name"] for m in r.json().get("models", [])]
        except Exception:
            return []


# ── Pretty printing ───────────────────────────────────────────────────────────
def print_banner():
    banner = """
╔═══════════════════════════════════════════════════════╗
║         Ollama Chat Demo — Memory System              ║
║  github.com/tuhin-su/ollama-master                    ║
╚═══════════════════════════════════════════════════════╝
    """
    if HAS_RICH:
        console.print(banner, style="bold blue")
    else:
        print(banner)


def print_info(label: str, value: str):
    if HAS_RICH:
        console.print(f"  [dim]{label}:[/dim] [cyan]{value}[/cyan]")
    else:
        print(f"  {label}: {value}")


def print_memory_note(memories: int):
    msg = f"🧠  {memories} memories injected from past sessions"
    if HAS_RICH:
        console.print(f"\n  [dim italic]{msg}[/dim italic]")
    else:
        print(f"\n  {msg}")


def print_user(text: str):
    if HAS_RICH:
        console.print(f"\n[bold green]You[/bold green]: {text}")
    else:
        print(f"\nYou: {text}")


def print_assistant_start(model: str):
    if HAS_RICH:
        console.print(f"\n[bold blue]{model}[/bold blue]: ", end="")
    else:
        print(f"\n{model}: ", end="", flush=True)


def print_token(token: str):
    if HAS_RICH:
        console.print(token, end="", markup=False)
    else:
        print(token, end="", flush=True)


def print_assistant_end():
    print()  # newline after streaming


def print_separator():
    if HAS_RICH:
        console.rule(style="dim")
    else:
        print("─" * 60)


def print_history(messages: list[dict]):
    if HAS_RICH:
        t = Table(title="Conversation History", show_header=True)
        t.add_column("Role",    style="cyan",  width=12)
        t.add_column("Content", style="white", no_wrap=False)
        for m in messages:
            role    = m["role"]
            img_suffix = f" [contains {len(m['images'])} image(s)]" if "images" in m else ""
            content = (m["content"][:120] + "…" if len(m["content"]) > 120 else m["content"]) + img_suffix
            color   = "green" if role == "user" else "blue"
            t.add_row(f"[{color}]{role}[/{color}]", content)
        console.print(t)
    else:
        print("\n─── Conversation History ───")
        for m in messages:
            img_suffix = f" [contains {len(m['images'])} image(s)]" if "images" in m else ""
            print(f"  [{m['role']}] {m['content'][:80]}{img_suffix}")
        print()


def print_help():
    commands = {
        "/help":    "Show this help",
        "/history": "Show conversation history",
        "/clear":   "Clear conversation history (memory is kept on server)",
        "/models":  "List available models",
        "/model":   "Switch model: /model llama3.2",
        "/system":  "Set system prompt: /system You are a pirate",
        "/image":   "Add image to next message: /image path/to/image.jpg",
        "/exit":    "Exit the demo",
    }
    if HAS_RICH:
        t = Table(title="Commands", show_header=False, box=None)
        t.add_column("Cmd",  style="cyan",  width=14)
        t.add_column("Desc", style="white")
        for cmd, desc in commands.items():
            t.add_row(cmd, desc)
        console.print(t)
    else:
        print("\n─── Commands ───")
        for cmd, desc in commands.items():
            print(f"  {cmd:14}  {desc}")
        print()


# ── Memory explanation panel ──────────────────────────────────────────────────
def explain_memory():
    text = """
[bold]How memory works in this demo:[/bold]

  1. You chat normally — the server handles everything
  2. After each reply, the server [green]extracts facts[/green] from the conversation
     (your name, projects, preferences, events)
  3. Facts are [green]embedded[/green] using your local embedding model and stored in LanceDB
  4. On the next request, the server [green]searches past memories[/green] and injects
     the most relevant ones into the system prompt automatically
  5. When you restart the script, memories from [yellow]previous sessions[/yellow] are loaded

[dim]To enable memory, add to ~/.ollama/server.json:[/dim]
  {
    "memory": {
      "enabled": true,
      "embedding_model": "nomic-embed-text"
    }
  }
"""
    if HAS_RICH:
        console.print(Panel(text, title="🧠 Memory System", border_style="blue"))
    else:
        print("\n─── Memory System ───")
        # Strip rich markup for plain mode
        import re
        plain = re.sub(r'\[.*?\]', '', text)
        print(plain)


# ── Main chat loop ────────────────────────────────────────────────────────────
def chat_loop(args: argparse.Namespace):
    client  = OllamaClient(args.host, args.token)
    model   = args.model
    history: list[dict] = []
    pending_images: list[str] = []
    system  = args.system

    print_banner()
    explain_memory()
    print_separator()

    # Health check
    if not client.check_health():
        if HAS_RICH:
            console.print(f"[red]✗ Cannot reach Ollama at {args.host}[/red]")
            console.print("[dim]  Start the server with: ollama serve[/dim]")
        else:
            print(f"✗ Cannot reach Ollama at {args.host}")
            print("  Start the server with: ollama serve")
        sys.exit(1)

    if HAS_RICH:
        console.print(f"[green]✓ Connected to {args.host}[/green]")
    else:
        print(f"✓ Connected to {args.host}")

    print_info("Model",   model)
    print_info("User ID", args.user or "auto (by IP)")
    print_info("Auth",    "Bearer token" if args.token else "none")
    print_info("Stream",  "yes" if not args.no_stream else "no")
    print_info("System",  system or "(none)")

    if HAS_RICH:
        console.print("\n[dim]Type [cyan]/help[/cyan] for commands, [cyan]/exit[/cyan] to quit[/dim]\n")
    else:
        print("\nType /help for commands, /exit to quit\n")

    print_separator()

    while True:
        # ── Get input ────────────────────────────────────────────────────────
        try:
            if HAS_RICH:
                user_input = Prompt.ask("\n[bold green]You[/bold green]").strip()
            else:
                user_input = input("\nYou: ").strip()
        except (KeyboardInterrupt, EOFError):
            print("\nExiting...")
            break

        if not user_input:
            continue

        # ── Commands ─────────────────────────────────────────────────────────
        if user_input.startswith("/"):
            cmd_parts = user_input.split(None, 1)
            cmd       = cmd_parts[0].lower()
            arg       = cmd_parts[1] if len(cmd_parts) > 1 else ""

            if cmd == "/exit":
                if HAS_RICH:
                    console.print("\n[dim]Goodbye! Your memories are saved.[/dim]")
                else:
                    print("\nGoodbye! Your memories are saved.")
                break

            elif cmd == "/help":
                print_help()

            elif cmd == "/history":
                if history:
                    print_history(history)
                else:
                    print("  No history yet.")

            elif cmd == "/clear":
                history = []
                if HAS_RICH:
                    console.print("[dim]  Conversation cleared. Server-side memories are preserved.[/dim]")
                else:
                    print("  Conversation cleared. Server-side memories are preserved.")

            elif cmd == "/models":
                models = client.list_models()
                if models:
                    if HAS_RICH:
                        for m in models:
                            console.print(f"  [cyan]•[/cyan] {m}")
                    else:
                        for m in models:
                            print(f"  • {m}")
                else:
                    print("  No models found (or server unreachable).")

            elif cmd == "/model":
                if arg:
                    model = arg
                    print_info("Switched model to", model)
                else:
                    print("  Usage: /model <name>")

            elif cmd == "/system":
                if arg:
                    system = arg
                    print_info("System prompt set to", system)
                else:
                    system = None
                    print("  System prompt cleared.")

            elif cmd == "/image":
                image_path = arg.strip()
                if image_path:
                    if (image_path.startswith('"') and image_path.endswith('"')) or (image_path.startswith("'") and image_path.endswith("'")):
                        image_path = image_path[1:-1]
                    if os.path.exists(image_path):
                        try:
                            encoded = encode_image(image_path)
                            pending_images.append(encoded)
                            if HAS_RICH:
                                console.print(f"[green]✓ Image loaded successfully:[/green] {image_path}")
                            else:
                                print(f"✓ Image loaded successfully: {image_path}")
                        except Exception as e:
                            print(f"  Error loading image: {e}")
                    else:
                        print(f"  File not found: {image_path}")
                else:
                    print("  Usage: /image <path/to/image>")

            else:
                print(f"  Unknown command: {cmd}. Type /help for commands.")

            continue

        # ── Normal chat ───────────────────────────────────────────────────────
        user_message = {"role": "user", "content": user_input}
        if pending_images:
            user_message["images"] = pending_images
            pending_images = []
        history.append(user_message)

        start_time  = time.time()
        full_reply  = ""
        token_count = 0

        print_assistant_start(model)

        try:
            if args.no_stream:
                full_reply = client.chat(model, history, system)
                print_token(full_reply)
                print_assistant_end()
            else:
                for token in client.chat_stream(model, history, system):
                    print_token(token)
                    full_reply  += token
                    token_count += 1
                print_assistant_end()

        except requests.exceptions.ConnectionError:
            print(f"\n✗ Connection lost. Is ollama serve running at {args.host}?")
            history.pop()   # remove the unanswered user message
            continue
        except requests.exceptions.HTTPError as e:
            if e.response is not None and e.response.status_code == 401:
                print("\n✗ 401 Unauthorized — set --token to match your server's api_token")
            else:
                print(f"\n✗ HTTP error: {e}")
            history.pop()
            continue
        except RuntimeError as e:
            print(f"\n✗ {e}")
            history.pop()
            continue

        # ── Stats ─────────────────────────────────────────────────────────────
        elapsed = time.time() - start_time
        if HAS_RICH:
            console.print(
                f"[dim]  ⏱ {elapsed:.1f}s  |  "
                f"~{len(full_reply.split())} words  |  "
                f"{datetime.now().strftime('%H:%M:%S')}[/dim]"
            )
        else:
            print(f"  ⏱ {elapsed:.1f}s | ~{len(full_reply.split())} words")

        # ── Add assistant reply to history ────────────────────────────────────
        if full_reply:
            history.append({"role": "assistant", "content": full_reply})

        print_separator()


# ── Entry point ───────────────────────────────────────────────────────────────
def main():
    parser = argparse.ArgumentParser(
        description="Ollama chat demo with native long-term memory",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python chat.py
  python chat.py --model llama3.2 --token mysecret
  python chat.py --system "You are a helpful Go expert"
  python chat.py --no-stream
        """,
    )
    parser.add_argument("--host",      default=os.getenv("OLLAMA_HOST", DEFAULT_HOST),
                        help="Ollama server URL (default: %(default)s)")
    parser.add_argument("--model",     default=os.getenv("OLLAMA_MODEL", DEFAULT_MODEL),
                        help="Model to use (default: %(default)s)")
    parser.add_argument("--token",     default=os.getenv("OLLAMA_TOKEN", ""),
                        help="Bearer token if api_token is set in server.json")
    parser.add_argument("--user",      default="",
                        help="User ID label (informational only; server uses IP)")
    parser.add_argument("--system",    default="",
                        help="System prompt override")
    parser.add_argument("--no-stream", action="store_true",
                        help="Disable streaming (wait for full reply)")

    args = parser.parse_args()
    if not args.system:
        args.system = None

    chat_loop(args)


if __name__ == "__main__":
    main()
