import json
import urllib.request
import urllib.error
import time
import sys
import random
import os
import subprocess

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

def run_system_verification_tests():
    print("\n==================================================")
    print("Running CLI System Verification Tests")
    print("==================================================")

    # Helper function to run local ollama command
    def run_cmd(args):
        cmd = ["./ollama"] + args
        try:
            res = subprocess.run(cmd, capture_output=True, text=True, check=True)
            return True, res.stdout, res.stderr
        except subprocess.CalledProcessError as e:
            return False, e.stdout, e.stderr

    # 1. Verify model command is registered and outputs help
    print("1. Testing 'ollama model --help'...")
    ok, stdout, stderr = run_cmd(["model", "--help"])
    if ok and "Manage Ollama models" in stdout:
        print(" [PASS] 'ollama model' registered successfully.")
    else:
        print(" [FAIL] 'ollama model' registration failed.")
        print(f"Stdout: {stdout}\nStderr: {stderr}")

    # 2. Verify export requires dest
    print("2. Testing 'ollama model' validation...")
    ok, stdout, stderr = run_cmd(["model", "--export", "testmodel"])
    if not ok and "dest is required" in stderr:
        print(" [PASS] model export validation works as expected.")
    else:
        print(" [FAIL] model export validation didn't error on missing dest.")
        print(f"Stdout: {stdout}\nStderr: {stderr}")

    # 3. Verify memory command is registered and has --import
    print("3. Testing 'ollama memory --help'...")
    ok, stdout, stderr = run_cmd(["memory", "--help"])
    if ok and "--import" in stdout and "--export" in stdout:
        print(" [PASS] 'ollama memory' subcommand and flags verified.")
    else:
        print(" [FAIL] 'ollama memory' subcommand or flags verification failed.")
        print(f"Stdout: {stdout}\nStderr: {stderr}")

    # 4. Verify memory export database operation
    print("4. Testing memory database export...")
    export_file = "test_memory_export.json"
    if os.path.exists(export_file):
        os.remove(export_file)
    ok, stdout, stderr = run_cmd(["memory", "--export", export_file, "--format", "json"])
    if ok and os.path.exists(export_file):
        print(f" [PASS] Memory successfully exported to {export_file}.")
        # Verify JSON content
        try:
            with open(export_file, "r") as f:
                data = json.load(f)
            if "conversations" in data:
                print(" [PASS] Memory export JSON structure is valid.")
            else:
                print(" [FAIL] Exported JSON is missing memory keys.")
        except Exception as e:
            print(f" [FAIL] Failed parsing exported memory JSON: {e}")
    else:
        print(" [FAIL] Memory database export failed.")
        print(f"Stdout: {stdout}\nStderr: {stderr}")

    # 5. Verify memory import database operation
    if os.path.exists(export_file):
        print("5. Testing memory database import...")
        ok, stdout, stderr = run_cmd(["memory", "--import", export_file])
        if ok and "Memory successfully imported" in stdout:
            print(" [PASS] Memory database import executed successfully.")
        else:
            print(" [FAIL] Memory database import failed.")
            print(f"Stdout: {stdout}\nStderr: {stderr}")
        os.remove(export_file)

    # 6. Verify skill command is registered and has --import/--export
    print("6. Testing 'ollama skill --help'...")
    ok, stdout, stderr = run_cmd(["skill", "--help"])
    if ok and "--import" in stdout and "--export" in stdout:
        print(" [PASS] 'ollama skill' subcommand verified.")
    else:
        print(" [FAIL] 'ollama skill' subcommand registration failed.")
        print(f"Stdout: {stdout}\nStderr: {stderr}")

    # 7. Verify skill export
    print("7. Testing skill export...")
    skills_dir = "test_skills_export"
    if os.path.exists(skills_dir):
        subprocess.run(["rm", "-rf", skills_dir])
    ok, stdout, stderr = run_cmd(["skill", "--export", skills_dir])
    if ok and os.path.exists(skills_dir):
        print(" [PASS] Skills successfully exported.")
        subprocess.run(["rm", "-rf", skills_dir])
    else:
        print(" [FAIL] Skill export failed (or no skills were present to export).")
        print(f"Stdout: {stdout}\nStderr: {stderr}")

def main():
    print("Ollama Memory Evaluator & CLI System Verification Test Suite")
    print("-----------------------------------------------------------")
    
    # Verify connection
    try:
        headers = {}
        if API_TOKEN:
            headers["Authorization"] = f"Bearer {API_TOKEN}"
        req = urllib.request.Request("http://localhost:11434", headers=headers)
        urllib.request.urlopen(req, timeout=3)
    except Exception:
        print("Warning: Could not connect to Ollama server at http://localhost:11434. Skipping connection-based memory benchmark.")
        # Proceed with offline CLI verification tests even if server is down
        run_system_verification_tests()
        sys.exit(0)

    print("Generating 10,000 unique QA pairs...")
    qa_pairs = generate_10k_qa_pairs()
    
    # Allow specifying how many items to sample-test in real-time from the CLI
    num_to_test = 5
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

    # Run the CLI system verification tests
    run_system_verification_tests()

if __name__ == "__main__":
    main()
