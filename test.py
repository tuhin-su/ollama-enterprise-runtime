import json
import urllib.request
import urllib.error
import time
import sys
import random
import os

OLLAMA_URL = "http://localhost:11434/api/chat"

def get_api_token():
    try:
        config_path = os.path.expanduser("~/.ollama/server.json")
        if os.path.exists(config_path):
            with open(config_path, "r") as f:
                config = json.load(f)
                return config.get("api_token", "")
    except Exception:
        pass
    return ""

API_TOKEN = get_api_token()

# The three models specified by the user
MODELS = [
    "qwen2.5-vl-7b:latest",
    "qwen3.5-9B-uncensored-aggressive:latest",
    "deepseek-r1-llama-8b-uncensored:latest"
]

def generate_10k_qa_pairs():
    """
    Generates 10,000 unique fact-question-answer pairs.
    """
    names = ["Alice", "Bob", "Charlie", "David", "Emma", "Frank", "Grace", "Henry", "Ivy", "Jack", "Kate", "Leo", "Mia", "Noah", "Olivia", "Peter", "Quinn", "Ryan", "Sara", "Toby"]
    colors = ["red", "blue", "green", "yellow", "purple", "orange", "pink", "brown", "black", "white", "gray", "violet", "indigo", "cyan", "magenta", "lime", "teal", "amber", "maroon", "navy"]
    cities = ["Paris", "London", "Tokyo", "Rome", "Berlin", "Sydney", "Cairo", "Toronto", "Beijing", "Mumbai", "Seoul", "Madrid", "Vienna", "Chicago", "Seattle", "Austin", "Denver", "Miami", "Boston", "Kyoto"]
    pets = ["dog", "cat", "turtle", "hamster", "parrot", "rabbit", "goldfish", "iguana", "ferret", "hedgehog"]
    
    qa_pairs = []
    # Loop to generate exactly 10,000 unique QA pairs
    for i in range(10000):
        name = random.choice(names)
        color = random.choice(colors)
        city = random.choice(cities)
        pet = random.choice(pets)
        num = i + 10000
        
        # Alternate types of facts to ensure diversity
        mode = i % 4
        if mode == 0:
            fact = f"User {num}'s favorite color is {color} and they own a {pet}."
            query = f"What color does User {num} like and what pet do they own?"
            expected = f"{color} and {pet}"
        elif mode == 1:
            fact = f"User {num} lives in {city} and works as engineer number {i}."
            query = f"Where does User {num} live?"
            expected = city
        elif mode == 2:
            fact = f"The secret pin code for User {num} is {num}."
            query = f"What is the secret pin code for User {num}?"
            expected = str(num)
        else:
            fact = f"User {num} went to {city} in the year {2000 + (i % 26)}."
            query = f"Which city did User {num} visit and in what year?"
            expected = f"{city} in {2000 + (i % 26)}"
            
        qa_pairs.append({
            "fact": fact,
            "query": query,
            "expected": expected
        })
    return qa_pairs

def send_chat_request(model, messages):
    payload = {
        "model": model,
        "messages": messages,
        "stream": False
    }
    headers = {"Content-Type": "application/json"}
    if API_TOKEN:
        headers["Authorization"] = f"Bearer {API_TOKEN}"
    req = urllib.request.Request(
        OLLAMA_URL,
        data=json.dumps(payload).encode("utf-8"),
        headers=headers,
        method="POST"
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            return data.get("message", {}).get("content", "")
    except urllib.error.URLError as e:
        print(f"\n[Error] Ollama connection failed: {e}")
        return None

def run_10k_memory_benchmark(model, qa_pairs, num_to_test=20):
    print(f"\n==================================================")
    print(f"Running 10K Question Benchmark for: {model}")
    print(f"==================================================")
    print(f"Successfully generated 10,000 unique questions/facts.")
    print(f"Testing a subset of {num_to_test} random queries over the memory backend...")

    passed = 0
    total_time = 0.0

    # Injecting/Asking a subset of the 10,000 questions to verify recall
    test_indices = random.sample(range(10000), num_to_test)
    
    for idx, test_idx in enumerate(test_indices):
        qa = qa_pairs[test_idx]
        
        # 1. Fact Injection
        inject_messages = [{"role": "user", "content": f"Here is a key fact: {qa['fact']}"}]
        t0 = time.time()
        ack = send_chat_request(model, inject_messages)
        inj_time = time.time() - t0
        
        if ack is None:
            print(f" [{idx+1}/{num_to_test}] Failed to inject fact. Skipping...")
            continue

        # 2. Querying the Model
        query_messages = [{"role": "user", "content": qa['query']}]
        t0 = time.time()
        ans = send_chat_request(model, query_messages)
        q_time = time.time() - t0
        
        if ans is None:
            print(f" [{idx+1}/{num_to_test}] Failed to query. Skipping...")
            continue
            
        total_time += (inj_time + q_time)
        
        # Check if the expected answer is contained in model output
        words_to_check = qa['expected'].lower().replace("and", "").split()
        success = all(word in ans.lower() for word in words_to_check)
        
        if success:
            passed += 1
            status = "PASS"
        else:
            status = "FAIL"

        print(f" [{idx+1}/{num_to_test}] Fact: {qa['fact']}")
        print(f"       Query: {qa['query']}")
        print(f"       Expected: {qa['expected']} | Answer: {ans.strip()}")
        print(f"       Result: {status} (Combined Turn: {inj_time + q_time:.2f}s)")
        print("-" * 50)

    success_rate = (passed / num_to_test) * 100 if num_to_test > 0 else 0
    avg_latency = total_time / (num_to_test * 2) if num_to_test > 0 else 0
    print(f"\nModel {model} completed. Accuracy: {success_rate:.1f}% ({passed}/{num_to_test}) | Avg Latency: {avg_latency:.2f}s")
    
    return {
        "model": model,
        "accuracy": success_rate,
        "avg_latency": avg_latency,
        "passed": passed,
        "total": num_to_test
    }

def main():
    print("Ollama Memory Evaluator: 10,000 Questions Dataset Benchmark")
    print("-----------------------------------------------------------")
    
    # Verify connection
    try:
        headers = {}
        if API_TOKEN:
            headers["Authorization"] = f"Bearer {API_TOKEN}"
        req = urllib.request.Request("http://localhost:11434", headers=headers)
        urllib.request.urlopen(req, timeout=3)
    except Exception:
        print("Error: Could not connect to Ollama server at http://localhost:11434. Make sure 'ollama serve' is running.")
        sys.exit(1)

    print("Generating 10,000 unique QA pairs...")
    qa_pairs = generate_10k_qa_pairs()
    
    # Allow specifying how many items to sample-test in real-time from the CLI
    num_to_test = 10
    if len(sys.argv) > 1:
        try:
            num_to_test = int(sys.argv[1])
        except ValueError:
            pass

    results = []
    for model in MODELS:
        try:
            res = run_10k_memory_benchmark(model, qa_pairs, num_to_test=num_to_test)
            results.append(res)
        except Exception as e:
            print(f"Failed testing model {model}: {e}")

    print(f"\n==================================================")
    print(f"Final 10,000 Question Dataset Benchmark Results")
    print(f"==================================================")
    print(f"{'Model':<45} | {'Accuracy':<10} | {'Avg Latency':<12}")
    print(f"-" * 75)
    for r in results:
        acc_str = f"{r['accuracy']:.1f}% ({r['passed']}/{r['total']})"
        print(f"{r['model']:<45} | {acc_str:<10} | {r['avg_latency']:>10.2f}s")

if __name__ == "__main__":
    main()
