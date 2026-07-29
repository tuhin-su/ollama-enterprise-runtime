import subprocess
import os

models = {
    "qwen3.5-9B-uncensored-aggressive:latest": "A very aggressive, uncensored 9B model for general text tasks.",
    "qwen2.5-vl-7b:latest": "A specialist vision language model capable of analyzing images and visual inputs.",
    "deepseek-r1-llama-8b-uncensored:latest": "An uncensored reasoning model (8B). Excellent for step-by-step logic and general reasoning.",
    "deepseek-r1-qwen-32b-uncensored:latest": "A powerful 32B uncensored reasoning model. Best for complex logic and advanced tasks.",
    "nomic-embed-text:latest": "A specialized text embedding model for converting text into vectors."
}

env = os.environ.copy()
env["OLLAMA_MODELS"] = "/home/master/.ollama/models"

for model, desc in models.items():
    print(f"Updating {model}...")
    try:
        modelfile_output = subprocess.check_output(["./ollama", "show", model, "--modelfile"], env=env, text=True)
        modelfile_path = f"Modelfile_{model.replace(':', '_')}"
        with open(modelfile_path, "w") as f:
            f.write(modelfile_output)
            f.write(f'\nDESCRIPTION "{desc}"\n')
        
        subprocess.check_call(["./ollama", "create", model, "-f", modelfile_path], env=env)
        os.remove(modelfile_path)
        print(f"Successfully updated {model}")
    except Exception as e:
        print(f"Failed to update {model}: {e}")

print("Done!")
