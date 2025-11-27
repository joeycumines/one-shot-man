# This is an example config.mk file, to support local customizations.

.DEFAULT_GOAL := all

ifeq ($(CUSTOM_TARGETS_DEFINED),)
CUSTOM_TARGETS_DEFINED := 1

# Define custom targets here.

endif
