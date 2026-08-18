# Creating a KernelQuest incident in 15 minutes

1. Create `scenarios/<slug>/scenario.yaml` with its ID, services, fault, validation commands, and three hints.
2. Add a minimal Dockerfile and only the assets the scenario needs.
3. Make the fault real and observable. The validation must test recovery itself rather than inspecting learner-provided text.
4. Build scenario images with `docker compose --profile scenario-build build`.
5. Add the scenario to the catalog and a `RuntimeManager` provisioning path, with labels, internal networking, and CPU/memory/PID limits.
6. Add parser, fault, validation, and E2E tests before publishing it.

The included `broken-nginx-upstream` incident intentionally routes Nginx to `api:8081` while the real API listens on `:8080`. Repairing `proxy_pass`, reloading Nginx, and verifying `/health` restores service.
