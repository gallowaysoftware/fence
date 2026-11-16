import json, glob, os, subprocess, re, statistics
def done(): return os.path.exists('/tmp/sweep.done')
print("=== sweep-stow (heuristic x 38 routes, gym-4a72ce2) ===")
if os.path.exists('var/league/sweep-stow/REPORT.md'):
    t=open('var/league/sweep-stow/REPORT.md').read()
    i=t.find('## Players')
    print(t[i:i+400])
print("\n=== sweep-stow-drift ===")
if os.path.exists('var/league/sweep-stow-drift'):
    subprocess.run(['python3','/tmp/compare_stow.py'])
print("\n=== w3 trade floor ===")
subprocess.run(['python3','/tmp/w3_report.py'])
