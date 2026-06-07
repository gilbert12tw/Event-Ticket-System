package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSSelfManagedK8sTrackDeclaresRequiredAssets(t *testing.T) {
	root := repoRoot(t)
	awsRoot := filepath.Join(root, "infra", "aws", "self-managed-k8s")

	for _, path := range []string{
		".env.aws.example",
		"README.md",
		filepath.Join("templates", "guardrails.yaml"),
		filepath.Join("templates", "cluster.yaml"),
	} {
		require.FileExists(t, filepath.Join(awsRoot, path))
	}

	for _, script := range []string{
		"00-preflight.sh",
		"10-cost-plan.sh",
		"20-deploy-guardrails.sh",
		"30-deploy-infra.sh",
		"40-bootstrap-k8s.sh",
		"50-build-and-deploy-cets.sh",
		"60-verify.sh",
		"70-failure-drill.sh",
		"72-reset-demo-db.sh",
		"80-prepare-release-gitops.sh",
		"82-verify-release-gitops.sh",
		"90-pause-cluster.sh",
		"91-resume-cluster.sh",
		"99-destroy-all.sh",
	} {
		path := filepath.Join(awsRoot, "scripts", script)
		require.FileExists(t, path)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(data), "#!/usr/bin/env bash"), "%s should be executable bash", script)
	}
}

func TestAWSSelfManagedK8sDoesNotCallNonAWSK8sTrack(t *testing.T) {
	root := repoRoot(t)
	content := readFilesUnderForArchitectureTest(t, filepath.Join(root, "infra", "aws", "self-managed-k8s"))

	for _, forbidden := range []string{
		"infra/k8s/baremetal",
		".env.baremetal",
		"kubectl_bm",
		"[baremetal]",
	} {
		assert.NotContains(t, content, forbidden)
	}
}

func TestAWSSelfManagedK8sCloudFormationStaysSelfManaged(t *testing.T) {
	root := repoRoot(t)
	templates := readFilesUnderForArchitectureTest(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", "templates"))

	for _, forbidden := range []string{
		"Type: AWS::EKS::Cluster",
		"Type: AWS::EKS::Nodegroup",
		"Type: AWS::RDS::DBInstance",
		"Type: AWS::ElastiCache::CacheCluster",
		"Type: AWS::EC2::NatGateway",
	} {
		assert.NotContains(t, templates, forbidden)
	}

	for _, required := range []string{
		"Type: AWS::EC2::LaunchTemplate",
		"Type: AWS::AutoScaling::AutoScalingGroup",
		"MixedInstancesPolicy:",
		"SpotAllocationStrategy: price-capacity-optimized",
		"WorkerAutoScalingGroup:",
		"Type: AWS::ECR::Repository",
		"AllowedValues: [1, 3]",
	} {
		assert.Contains(t, templates, required)
	}
}

func TestAWSSelfManagedK8sDefaultsAreBudgetFirst(t *testing.T) {
	root := repoRoot(t)
	env := readArchitectureText(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", ".env.aws.example"))
	lib := readArchitectureText(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", "scripts", "lib.sh"))
	cluster := readArchitectureText(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", "templates", "cluster.yaml"))
	costPlan := readArchitectureText(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", "scripts", "10-cost-plan.sh"))

	for _, fragment := range []string{
		"AWS_REGION=us-west-2",
		"AWS_ALLOWED_REGIONS=us-west-2,us-east-2,us-east-1",
		"NODE_COUNT=3",
		"CONTROL_PLANE_INSTANCE_TYPE=r6a.large",
		"FALLBACK_INSTANCE_TYPE_1=r5a.large",
		"FALLBACK_INSTANCE_TYPE_2=r6i.large",
		"PURCHASE_OPTION=OnDemand",
		"APP_INGRESS_MODE=cloudflare",
		"BUDGET_LIMIT_USD=190",
		"COST_RESERVE_USD=15",
		"MAX_RUNTIME_HOURS=336",
		"ACTIVE_RUNTIME_HOURS=336",
		"K8S_CONTAINER_LOG_MAX_SIZE=50Mi",
		"K8S_CONTAINER_LOG_MAX_FILES=10",
		"FORCE_PRIVATE_IMAGE_ROLLOUT=false",
		"CETS_IMAGE_PLATFORM=linux/amd64",
	} {
		assert.Contains(t, env, fragment)
	}
	assert.Contains(t, lib, "AWS_REGION=${AWS_REGION:-us-west-2}")
	assert.Contains(t, lib, "AWS_ALLOWED_REGIONS=${AWS_ALLOWED_REGIONS:-us-west-2,us-east-2,us-east-1}")
	assert.Contains(t, cluster, "AppLoadBalancer:")
	assert.Contains(t, cluster, "Condition: UseAppNlb")
	assert.Contains(t, cluster, "ApiLoadBalancer:")
	assert.Contains(t, cluster, "Condition: ThreeNode")
	assert.Contains(t, cluster, "load_balancing.cross_zone.enabled")
	assert.Contains(t, cluster, "preserve_client_ip.enabled")
	assert.Contains(t, cluster, "VpcApiIngress:")
	assert.Contains(t, cluster, "CidrIp: !Ref VpcCidr")
	assert.Contains(t, costPlan, "aws pricing get-products")
	assert.Contains(t, costPlan, "describe-spot-price-history")
	assert.Contains(t, costPlan, "r6a.large")
	assert.Contains(t, costPlan, "r5a.large")
	assert.Contains(t, costPlan, "r6i.large")
	assert.Contains(t, costPlan, "max_instance_price")
	assert.Contains(t, costPlan, "validate_instance_type_overrides")
	assert.Contains(t, costPlan, "ACTIVE_RUNTIME_HOURS")
	assert.Contains(t, costPlan, "ESTIMATED_ACTIVE_HOURS")
	assert.Contains(t, costPlan, "ESTIMATED_STACK_HOURS")
	assert.Contains(t, costPlan, `if [ "$APP_INGRESS_MODE" = "nlb" ]`)
	assert.Contains(t, costPlan, "estimated cost")
}

func TestAWSSelfManagedK8sGuardrailsDeleteTaggedStacks(t *testing.T) {
	root := repoRoot(t)
	guardrails := readArchitectureText(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", "templates", "guardrails.yaml"))
	cluster := readArchitectureText(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", "templates", "cluster.yaml"))
	destroy := readArchitectureText(t, filepath.Join(root, "infra", "aws", "self-managed-k8s", "scripts", "99-destroy-all.sh"))

	for _, fragment := range []string{
		"Type: AWS::Budgets::Budget",
		"Type: AWS::SNS::Topic",
		"Type: AWS::Lambda::Function",
		"Type: AWS::Events::Rule",
		"rate(1 hour)",
		"cloudformation:DeleteStack",
		"autoscaling:SetDesiredCapacity",
		"ecr:BatchDeleteImage",
		"IncludeCredit: false",
		"CetsCleanupGroup",
		"TRACK_VALUE = \"aws-self-managed-k8s\"",
		"name.startswith(PREFIX)",
		"tags.get(TRACK_TAG) == TRACK_VALUE",
		"if not in_scope(name, tags) or not name.startswith(f\"{PREFIX}-\")",
		"if not in_scope(name, tags) or not name.startswith(f\"{PREFIX}/\")",
		"delete_stack",
	} {
		assert.Contains(t, guardrails, fragment)
	}
	assert.GreaterOrEqual(t, strings.Count(cluster, "Key: Track"), 4)
	assert.Contains(t, cluster, "PropagateAtLaunch: true")
	assert.Contains(t, destroy, "list-tags-for-resource")
	assert.Contains(t, destroy, "stack_tag_value")
	assert.Contains(t, destroy, "cloudformation describe-stacks")
	assert.Contains(t, destroy, "tags[?Key=='Track'].Value")
	assert.Contains(t, destroy, `asg_name" != "$AWS_STACK_PREFIX-"*`)
	assert.Contains(t, destroy, `track" != "aws-self-managed-k8s"`)
	assert.Contains(t, destroy, "skipping stack")
}

func TestAWSSelfManagedK8sScriptsCloseReviewerLandmines(t *testing.T) {
	root := repoRoot(t)
	awsRoot := filepath.Join(root, "infra", "aws", "self-managed-k8s")
	env := readArchitectureText(t, filepath.Join(awsRoot, ".env.aws.example"))
	deployInfra := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "30-deploy-infra.sh"))
	bootstrap := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "40-bootstrap-k8s.sh"))
	nodeAPIProxy := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "bootstrap-k8s", "node-api-proxy.sh"))
	verify := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "60-verify.sh"))
	drill := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "70-failure-drill.sh"))
	images := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "deploy-cets", "images.sh"))
	app := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "deploy-cets", "app.sh"))
	platform := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "deploy-cets", "platform.sh"))

	assert.Contains(t, env, "DEDICATED_AWS_ACCOUNT_ACK=false")
	assert.Contains(t, deployInfra, "guardrails stack")
	assert.Contains(t, deployInfra, "ExpirationUtc")
	assert.Contains(t, bootstrap, `API_ENDPOINT="${PUBLIC_IPS[0]}:6443"`)
	assert.Contains(t, bootstrap, "single-node mode requires a public IP")
	assert.Contains(t, bootstrap, "remote_has_file")
	assert.Contains(t, bootstrap, "bash <<'CETS_SSM_SCRIPT'")
	assert.Contains(t, bootstrap, "gpg --dearmor --yes")
	assert.Contains(t, bootstrap, "wait_for_api_target_healthy")
	assert.Contains(t, bootstrap, "bootstrap-k8s/node-api-proxy.sh")
	assert.Contains(t, bootstrap, "NODE_LOCAL_API_ENDPOINT=\"$API_HOST:6444\"")
	assert.Contains(t, bootstrap, "JOIN_KUBECONFIG=/tmp/kubeadm-bootstrap.conf")
	assert.Contains(t, bootstrap, "--kubeconfig $JOIN_KUBECONFIG")
	assert.Contains(t, bootstrap, "patch configmap cluster-info")
	assert.Contains(t, bootstrap, "patch configmap kubeadm-config")
	assert.Contains(t, bootstrap, "using node-local API proxy for kubeadm join discovery")
	assert.Contains(t, bootstrap, "failed to render node-local API proxy command")
	assert.Contains(t, bootstrap, "pointing kube-proxy at node-local API proxy")
	assert.Contains(t, bootstrap, "patch configmap kube-proxy")
	assert.Contains(t, bootstrap, "hostAliases")
	assert.Contains(t, bootstrap, "reconcile_stale_nodes")
	assert.Contains(t, bootstrap, "InternalIP")
	assert.Contains(t, bootstrap, "removing stale node")
	assert.Contains(t, bootstrap, "MISSING_NODE_NAMES")
	assert.Contains(t, bootstrap, "no missing Kubernetes node name is available")
	assert.Contains(t, bootstrap, "kubectl --request-timeout=10s get nodes")
	assert.Contains(t, bootstrap, "validate_k8s_log_rotation")
	assert.Contains(t, bootstrap, `cert_sans="    - $API_HOST"`)
	assert.Contains(t, bootstrap, `JOIN_COMMAND=${JOIN_COMMAND/"$API_ENDPOINT"/"$NODE_LOCAL_API_ENDPOINT"}`)
	assert.Contains(t, bootstrap, "containerLogMaxSize")
	assert.Contains(t, bootstrap, "containerLogMaxFiles")
	assert.NotContains(t, bootstrap, "mapfile")
	assert.Contains(t, bootstrap, "Calico installation already exists")
	assert.Contains(t, nodeAPIProxy, "haproxy")
	assert.Contains(t, nodeAPIProxy, "awk -v host")
	assert.Contains(t, nodeAPIProxy, `tmp_hosts=\$(mktemp)`)
	assert.Contains(t, nodeAPIProxy, "127.0.0.1 %s")
	assert.Contains(t, nodeAPIProxy, "bind 127.0.0.1:6444")
	assert.Contains(t, nodeAPIProxy, "$API_HOST")
	assert.Contains(t, verify, "member list")
	assert.Contains(t, verify, "started etcd members")
	assert.Contains(t, drill, "remove_etcd_member")
	assert.Contains(t, drill, "member remove")
	assert.Contains(t, app, "jq -nr")
	assert.Contains(t, app, "@uri")
	assert.Contains(t, app, "create secret generic cets-runtime-env")
	assert.Contains(t, app, "create_ecr_pull_secret")
	assert.Contains(t, app, "ecr_registry_from_image")
	assert.Contains(t, app, "CETS_API_IMAGE and CETS_FRONTEND_IMAGE must use the same ECR registry")
	assert.Contains(t, app, "resolve_ecr_image_digest_ref")
	assert.Contains(t, app, "aws_cli ecr describe-images")
	assert.Contains(t, app, "imageDetails[0].imageDigest")
	assert.Contains(t, app, "api_deploy_image")
	assert.Contains(t, app, "frontend_deploy_image")
	assert.Contains(t, app, "aws_cli ecr get-login-password")
	assert.Contains(t, app, "kubernetes.io/dockerconfigjson")
	assert.Contains(t, app, "--from-file=.dockerconfigjson")
	assert.Contains(t, app, "imagePullSecrets:")
	assert.Contains(t, app, "- name: cets-ecr-pull")
	assert.Contains(t, app, "restart_private_image_deployments_if_requested")
	assert.Contains(t, app, "FORCE_PRIVATE_IMAGE_ROLLOUT")
	assert.Contains(t, app, "rollout restart")
	assert.Contains(t, app, "wait_private_image_deployments")
	assert.Contains(t, app, "worker-export")
	assert.Contains(t, app, `rm -f "$tmpfile"`)
	assert.NotContains(t, app, "--docker-password")
	assert.NotContains(t, app, `DATABASE_URL: "postgresql://$POSTGRES_USER:$POSTGRES_PASSWORD`)
	assert.Contains(t, images, `docker build --platform "$CETS_IMAGE_PLATFORM"`)
	assert.Contains(t, images, "for $CETS_IMAGE_PLATFORM")
	assert.Contains(t, platform, "create secret generic cets-pg-app")
	assert.NotContains(t, platform, "stringData:\n  username: $POSTGRES_USER")
}

func TestAWSSelfManagedK8sPauseResumeContract(t *testing.T) {
	root := repoRoot(t)
	awsRoot := filepath.Join(root, "infra", "aws", "self-managed-k8s")
	env := readArchitectureText(t, filepath.Join(awsRoot, ".env.aws.example"))
	pause := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "90-pause-cluster.sh"))
	resume := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "91-resume-cluster.sh"))
	spec := readArchitectureText(t, filepath.Join(root, "docs", "specs", "aws-self-managed-k8s.md"))
	readme := readArchitectureText(t, filepath.Join(awsRoot, "README.md"))

	assert.Contains(t, env, "PAUSE_DESTROYS_NODE_LOCAL_DATA_ACK=false")
	assert.Contains(t, env, "RESUME_DEPLOY_APP=false")
	assert.Contains(t, pause, "require_apply")
	assert.Contains(t, pause, "PAUSE_DESTROYS_NODE_LOCAL_DATA_ACK")
	assert.Contains(t, pause, "assert_stack_in_scope")
	assert.Contains(t, pause, "assert_asg_in_scope")
	assert.Contains(t, pause, "ControlPlaneAutoScalingGroupName")
	assert.Contains(t, pause, "WorkerAutoScalingGroupName")
	assert.Contains(t, pause, "--min-size 0")
	assert.Contains(t, pause, "--desired-capacity 0")
	assert.Contains(t, pause, `rm -f "$GENERATED_DIR/kubeconfig"`)
	assert.NotContains(t, pause, "rm -rf")
	assert.NotContains(t, pause, "infra/k8s/baremetal")

	assert.Contains(t, resume, "10-cost-plan.sh")
	assert.Contains(t, resume, "require_fresh_guardrails")
	assert.Contains(t, resume, "cluster_stack_needs_reconcile")
	assert.Contains(t, resume, "stack_parameter_value")
	assert.Contains(t, resume, "30-deploy-infra.sh")
	assert.Contains(t, resume, "--desired-capacity \"$NODE_COUNT\"")
	assert.Contains(t, resume, "40-bootstrap-k8s.sh")
	assert.Contains(t, resume, "RESUME_DEPLOY_APP")
	assert.NotContains(t, resume, "infra/k8s/baremetal")

	for _, fragment := range []string{
		"destructive",
		"node-local",
		"ACTIVE_RUNTIME_HOURS",
		"NLB x MAX_RUNTIME_HOURS",
		"IncludeCredit:\n  false",
		"K8S_CONTAINER_LOG_MAX_SIZE",
		"Pause scales only in-scope AWS self-managed ASGs",
		"Resume reruns cost preflight",
	} {
		assert.Contains(t, spec, fragment)
	}
	assert.Contains(t, readme, "Spot is a cost option")
	assert.Contains(t, readme, "r6a.large")
	assert.Contains(t, readme, "r5a.large")
	assert.Contains(t, readme, "r6i.large")
	assert.Contains(t, readme, "On-Demand nodes")
	assert.Contains(t, readme, "r6a.xlarge")
}

func TestAWSSelfManagedK8sValidatesKubeletLogRotation(t *testing.T) {
	root := repoRoot(t)
	lib := filepath.Join(root, "infra", "aws", "self-managed-k8s", "scripts", "lib.sh")

	run := func(size, files string) (string, error) {
		t.Helper()
		cmd := exec.Command(
			"bash",
			"-c",
			`. "$1"; load_env; K8S_CONTAINER_LOG_MAX_SIZE="$2"; K8S_CONTAINER_LOG_MAX_FILES="$3"; validate_k8s_log_rotation`,
			"aws-log-rotation-test",
			lib,
			size,
			files,
		)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	output, err := run("50Mi", "10")
	require.NoError(t, err, output)

	output, err = run("50MB", "10")
	require.Error(t, err)
	assert.Contains(t, output, "binary size suffix")

	output, err = run("abc", "10")
	require.Error(t, err)
	assert.Contains(t, output, "positive integer")

	output, err = run("50Mi", "abc")
	require.Error(t, err)
	assert.Contains(t, output, "positive integer")

	output, err = run("50Mi", "0")
	require.Error(t, err)
	assert.Contains(t, output, "must be >= 1")
}

func TestAWSSelfManagedK8sCostPlanActiveHoursOffline(t *testing.T) {
	root := repoRoot(t)
	awsRoot := filepath.Join(root, "infra", "aws", "self-managed-k8s")
	generatedCostPlan := filepath.Join(awsRoot, "generated", "cost-plan.env")
	originalCostPlan, readErr := os.ReadFile(generatedCostPlan)
	existed := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		require.NoError(t, readErr)
	}
	t.Cleanup(func() {
		if existed {
			require.NoError(t, os.WriteFile(generatedCostPlan, originalCostPlan, 0o600))
		} else {
			require.NoError(t, os.RemoveAll(generatedCostPlan))
		}
	})

	output, err := runCostPlanWithStubAWS(t, root, map[string]string{
		"ACTIVE_RUNTIME_HOURS": "100",
		"MAX_RUNTIME_HOURS":    "336",
		"BUDGET_LIMIT_USD":     "190",
		"COST_RESERVE_USD":     "15",
	})
	require.NoError(t, err, output)
	assert.Contains(t, output, "active_hours=100")
	assert.Contains(t, output, "stack_hours=336")
	planned := readArchitectureText(t, generatedCostPlan)
	assert.Contains(t, planned, "ESTIMATED_TOTAL_USD=64.61")
	assert.Contains(t, planned, "ESTIMATED_ACTIVE_HOURS=100")
	assert.Contains(t, planned, "ESTIMATED_STACK_HOURS=336")

	output, err = runCostPlanWithStubAWS(t, root, map[string]string{
		"ACTIVE_RUNTIME_HOURS": "337",
		"MAX_RUNTIME_HOURS":    "336",
	})
	require.Error(t, err)
	assert.Contains(t, output, "must be <= MAX_RUNTIME_HOURS")

	output, err = runCostPlanWithStubAWS(t, root, map[string]string{
		"ACTIVE_RUNTIME_HOURS": "100",
		"MAX_RUNTIME_HOURS":    "336",
		"BUDGET_LIMIT_USD":     "50",
		"COST_RESERVE_USD":     "15",
	})
	require.Error(t, err)
	assert.Contains(t, output, "estimated cost")
	assert.Contains(t, output, "exceeds deploy limit")

	output, err = runCostPlanWithStubAWS(t, root, map[string]string{
		"CONTROL_PLANE_INSTANCE_TYPE": "m6a.2xlarge",
		"FALLBACK_INSTANCE_TYPE_1":    "m6a.2xlarge",
		"FALLBACK_INSTANCE_TYPE_2":    "m7i.2xlarge",
	})
	require.Error(t, err)
	assert.Contains(t, output, "must be distinct")
}

func TestAWSSelfManagedK8sArgoCDReleaseBranchContract(t *testing.T) {
	root := repoRoot(t)
	awsRoot := filepath.Join(root, "infra", "aws", "self-managed-k8s")
	gitops := readFilesUnderForArchitectureTest(t, filepath.Join(awsRoot, "gitops"))
	app := readArchitectureText(t, filepath.Join(awsRoot, "gitops", "argocd", "application.yaml"))
	kustomization := readArchitectureText(t, filepath.Join(awsRoot, "gitops", "app", "kustomization.yaml"))
	prepare := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "80-prepare-release-gitops.sh"))
	verify := readArchitectureText(t, filepath.Join(awsRoot, "scripts", "82-verify-release-gitops.sh"))
	releaseWorkflow := readArchitectureText(t, filepath.Join(root, ".github", "workflows", "aws-k8s-release.yml"))
	spec := readArchitectureText(t, filepath.Join(root, "docs", "specs", "aws-self-managed-k8s.md"))

	for _, path := range []string{
		filepath.Join("gitops", "app", "backend.yaml"),
		filepath.Join("gitops", "app", "frontend.yaml"),
		filepath.Join("gitops", "app", "workers.yaml"),
		filepath.Join("gitops", "app", "migrate-job.yaml"),
		filepath.Join("gitops", "argocd", "application.yaml"),
	} {
		require.FileExists(t, filepath.Join(awsRoot, path))
	}

	assert.Contains(t, app, "kind: Application")
	assert.Contains(t, app, "targetRevision: release/aws-self-managed-k8s")
	assert.Contains(t, app, "path: infra/aws/self-managed-k8s/gitops/app")
	assert.NotContains(t, app, "automated:")
	assert.Contains(t, kustomization, "newTag: v0.0.1")
	assert.NotContains(t, kustomization, "latest")
	assert.Contains(t, prepare, "kustomize")
	assert.Contains(t, prepare, "CETS_RELEASE_VERSION must look like v0.0.1")
	assert.Contains(t, prepare, "ALLOW_MISSING_RELEASE_TAG=true")
	assert.Contains(t, verify, "expected both app images to use newTag")
	assert.Contains(t, verify, "assert_release_tag_at_head")
	assert.Contains(t, verify, "release branch HEAD must be tagged")
	assert.Contains(t, verify, "kind:[[:space:]]*Secret")
	assert.Contains(t, verify, "GitOps manifests must not contain unsealed Kubernetes Secrets")

	for _, forbidden := range []string{
		"stringData:",
		"DATABASE_URL:",
		"POSTGRES_PASSWORD",
		"TOKEN_SIGNING_SECRET",
		"PROVIDER_TOKEN_SECRET",
		"BOOKING_RESERVATION_HASH_SECRET",
	} {
		assert.NotContains(t, gitops, forbidden)
	}

	assert.Contains(t, releaseWorkflow, "release/aws-self-managed-k8s")
	assert.Contains(t, releaseWorkflow, "pull_request:")
	assert.Contains(t, releaseWorkflow, `- "v*"`)
	assert.Contains(t, releaseWorkflow, "infra/aws/self-managed-k8s/**")
	assert.Contains(t, releaseWorkflow, "github.ref == 'refs/heads/release/aws-self-managed-k8s'")
	assert.Contains(t, releaseWorkflow, "startsWith(github.ref, 'refs/tags/v')")
	assert.Contains(t, releaseWorkflow, "82-verify-release-gitops.sh")
	assert.NotContains(t, releaseWorkflow, "argocd app sync")
	assert.NotContains(t, releaseWorkflow, "kubectl apply")
	assert.Contains(t, spec, "Automated sync is not enabled")
	assert.Contains(t, spec, "matching tag to point at release branch HEAD")
}

func TestAWSSelfManagedK8sDocsKeepPhaseBoundaries(t *testing.T) {
	root := repoRoot(t)
	spec := readArchitectureText(t, filepath.Join(root, "docs", "specs", "aws-self-managed-k8s.md"))
	architecture := readArchitectureText(t, filepath.Join(root, "docs", "ARCHITECTURE.md"))

	assert.Contains(t, spec, "It is not EKS")
	assert.Contains(t, spec, "No claim that Kubernetes is a Phase 1 or Phase 2 deliverable.")
	assert.Contains(t, architecture, "AWS Self-Managed Kubernetes Experiment")
	assert.Contains(t, architecture, "不使用 EKS")
	assert.Contains(t, architecture, "不取代 Phase 3 local Compose")
	assert.Contains(t, architecture, "Phase 1 或 Phase 2")
}
