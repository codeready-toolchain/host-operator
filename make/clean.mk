.PHONY: clean
clean:
	$(Q)-rm -rf ${V_FLAG} $(OUT_DIR) ./vendor
	$(Q)go clean ${X_FLAG} ./...
	$(Q)-rm deploy/templates/nstemplatetiers/metadata.yaml 2>/dev/null || true
	$(Q)-rm deploy/templates/nstemplatetiers/namespaces/metadata.yaml 2>/dev/null || true
	$(Q)-rm deploy/templates/nstemplatetiers/clusterresources/metadata.yaml 2>/dev/null || true
	$(Q)-rm pkg/templates/nstemplatetiers/nstemplatetier_assets.go 2>/dev/null || true
	$(Q)-rm test/templates/nstemplatetiers/nstemplatetier_assets.go 2>/dev/null || true
	$(Q)-rm test/templates/nstemplatetiers/namespaces/namespaces_assets.go || true
	$(Q)-rm test/templates/nstemplatetiers/clusterresources/clusterresources_assets.go || true
