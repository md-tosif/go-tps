#!/usr/bin/env python3
# Wrapper script for graph_metrics.py
# Generates TPS and Latency graphs, saves to images/ folder
import os
import sys

script_dir = os.path.dirname(os.path.abspath(__file__))
actual_script = os.path.join(script_dir, 'scripts', 'graph_metrics.py')

# Execute the actual script
os.execv(sys.executable, [sys.executable, actual_script] + sys.argv[1:])
