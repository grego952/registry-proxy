package resources

import (
	"os"

	"github.tools.sap/kyma/image-pull-reverse-proxy/components/controller/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

const (
	defaultLimitCPU      = "100m"
	defaultLimitMemory   = "128Mi"
	defaultRequestCPU    = "100m"
	defaultRequestMemory = "128Mi"
	reverseProxyPort     = 8080
	probesPort           = 8081
)

type deployment struct {
	reverseProxy *v1alpha1.ImagePullReverseProxy
}

func NewDeployment(rp *v1alpha1.ImagePullReverseProxy) *appsv1.Deployment {
	d := &deployment{
		reverseProxy: rp,
	}
	return d.construct()
}

func (d *deployment) construct() *appsv1.Deployment {
	deploymentLabels := labels(d.reverseProxy, "deployment")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.reverseProxy.Name,
			Namespace: d.reverseProxy.Namespace,
			Labels:    labels(d.reverseProxy, "deployment"),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: deploymentLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: deploymentLabels,
				},
				Spec: d.podSpec(),
			},
			Replicas: ptr.To[int32](1),
		},
	}
	return deployment
}

func (d *deployment) podSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  d.reverseProxy.Name,
				Image: os.Getenv("PROXY_IMAGE"),
				Command: []string{
					os.Getenv("PROXY_COMMAND"),
				},
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       d.resourceConfiguration(),
				Env:             d.envs(),
				Ports: []corev1.ContainerPort{
					{
						// TODO: odsztywinić?
						ContainerPort: reverseProxyPort,
						Protocol:      "TCP",
					},
				},
				StartupProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.FromInt32(probesPort),
						},
					},
					InitialDelaySeconds: 0,
					PeriodSeconds:       5,
					SuccessThreshold:    1,
					FailureThreshold:    30, // FailureThreshold * PeriodSeconds = 150s in this case, this should be enough for any function pod to start up
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/readyz",
							Port: intstr.FromInt32(probesPort),
						},
					},
					InitialDelaySeconds: 0, // startup probe exists, so delaying anything here doesn't make sense
					FailureThreshold:    1,
					PeriodSeconds:       5,
					TimeoutSeconds:      2,
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.FromInt32(probesPort),
						},
					},
					FailureThreshold: 3,
					PeriodSeconds:    5,
					TimeoutSeconds:   4,
				},
				SecurityContext: &corev1.SecurityContext{
					RunAsGroup: d.podRunAsUserUID(), // set to 1000 because default value is root(0)
					RunAsUser:  d.podRunAsUserUID(),
					SeccompProfile: &corev1.SeccompProfile{
						Type: corev1.SeccompProfileTypeRuntimeDefault,
					},
					AllowPrivilegeEscalation: ptr.To[bool](false),
					RunAsNonRoot:             ptr.To[bool](true),
					Capabilities: &corev1.Capabilities{
						Drop: []corev1.Capability{
							"All",
						},
					},
				},
			},
		},
	}
}

func (d *deployment) resourceConfiguration() corev1.ResourceRequirements {
	if d.reverseProxy.Spec.Resources != nil {
		return *d.reverseProxy.Spec.Resources
	}

	return defaultResources()
}

func defaultResources() corev1.ResourceRequirements {
	// TODO: adjust
	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(defaultLimitCPU),
			corev1.ResourceMemory: resource.MustParse(defaultLimitMemory),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(defaultRequestCPU),
			corev1.ResourceMemory: resource.MustParse(defaultRequestMemory),
		},
	}
}

func (d *deployment) envs() []corev1.EnvVar {
	envVariables := []corev1.EnvVar{
		{
			Name:  "PROXY_URL",
			Value: d.reverseProxy.Spec.ProxyURL,
		},
		{
			Name:  "TARGET_HOST",
			Value: d.reverseProxy.Spec.TargetHost,
		},
	}

	if d.reverseProxy.Spec.LogLevel != "" {
		envVariables = append(envVariables, corev1.EnvVar{
			Name:  "LOG_LEVEL",
			Value: d.reverseProxy.Spec.LogLevel,
		})
	}

	return envVariables
}

func (d *deployment) podRunAsUserUID() *int64 {
	return ptr.To[int64](1000) // runAsUser 1000 is the most popular and standard value for non-root user
}
