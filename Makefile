.PHONY: test manifests check proto

test:
	$(MAKE) -C app test
	python -m unittest discover -s scripts/tests -v

proto:
	$(MAKE) -C app proto

manifests:
	kubectl kustomize gitops/apps/overlays/dev > /dev/null
	kubectl kustomize gitops/apps/overlays/staging > /dev/null
	kubectl kustomize gitops/apps/overlays/prod > /dev/null

check: test manifests
	terraform fmt -check -recursive iac
	python scripts/check_template.py
