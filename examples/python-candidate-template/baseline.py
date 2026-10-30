import argparse
from pathlib import Path


parser = argparse.ArgumentParser()
parser.add_argument("--iterations", type=int, required=True)
parser.add_argument("--input")
args = parser.parse_args()

value = 0
if args.input:
    value = sum(int(line) for line in Path(args.input).read_text(encoding="utf-8").splitlines())

for index in range(args.iterations):
    value = (value * 31 + index + 7) % 1_000_000_007

print(value)
