.PHONY: clean
clean:
	$(Q)-rm -rf ${V_FLAG} $(OUT_DIR) ./vendor
	$(Q)go clean ${X_FLAG} ./...
	$(Q)-rm pkg/templates/template_contents.go
	$(Q)-rm deploy/nstemplatetiers/metadata.yaml
