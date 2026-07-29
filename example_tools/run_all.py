import subprocess
import sys
import os
import signal
import time

scripts = ["python_sandbox.py", "data_analyzer.py", "web_retriever.py", "system_resources.py"]
processes = []

def cleanup(sig, frame):
    print("\nTerminating all active tools...")
    for p in processes:
        p.terminate()
    sys.exit(0)

signal.signal(signal.SIGINT, cleanup)
signal.signal(signal.SIGTERM, cleanup)

def main():
    dir_path = os.path.dirname(os.path.realpath(__file__))
    
    print("Starting all example tools concurrently...")
    for script in scripts:
        script_path = os.path.join(dir_path, script)
        p = subprocess.Popen([sys.executable, script_path])
        processes.append(p)
        print(f"Launched {script} (PID: {p.pid})")
        
    print("\nAll tools running. Press Ctrl+C to terminate.")
    while True:
        time.sleep(1)

if __name__ == "__main__":
    main()
