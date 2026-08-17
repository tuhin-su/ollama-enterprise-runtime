#!/bin/bash

# Ensure loom is built and available
if [ ! -f "./loom" ]; then
    echo "Loom binary not found. Please build it first."
    exit 1
fi

echo "Importing DeepSeek-R1-0528-Qwen3-8B-Q4_K_M.gguf..."
./loom model --import ~/.models/DeepSeek-R1-0528-Qwen3-8B-Q4_K_M.gguf \
    --description "DeepSeek-R1 reasoning model based on Qwen3 8B architecture. Capabilities: Advanced logic, reasoning, mathematics, and problem-solving. Rating: 4.5/5 for complex reasoning tasks."

echo "Importing Qwen2.5-Coder-7B-Instruct-Q4_K_M.gguf..."
./loom model --import ~/.models/Qwen2.5-Coder-7B-Instruct-Q4_K_M.gguf \
    --description "Qwen2.5 Coder 7B Instruct model. Capabilities: State-of-the-art code generation, debugging, and technical writing. Rating: 4.8/5 for software development tasks."

echo "Finished importing models!"
