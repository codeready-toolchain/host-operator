# Create output directory for artifacts and test results. ./out is supposed to
# be a safe place for all targets to write to while knowing that all content
# inside of ./out is wiped once "make clean" is run.
OUT_DIR := ./out
$(shell mkdir -p $(OUT_DIR));