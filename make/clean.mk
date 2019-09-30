.PHONY: clean
clean:
	$(Q)-rm -rf ${V_FLAG} $(OUT_DIR) ./vendor
	$(Q)go clean ${X_FLAG} ./...
	$(Q)-rm pkg/templates/nstemplatetiers/nstemplatetier_assets.go > /dev/null
	$(Q)-rm deploy/templates/nstemplatetiers/metadata.yaml > /dev/null
