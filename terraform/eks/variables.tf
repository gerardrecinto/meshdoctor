variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-west-2"
}

variable "name" {
  description = "Cluster name, also used as the VPC name"
  type        = string
  default     = "meshdoctor"
}

variable "kubernetes_version" {
  type    = string
  default = "1.31"
}

variable "istio_version" {
  description = "Istio chart version for base, istiod and the ingress gateway"
  type        = string
  default     = "1.24.2"
}

variable "node_instance_types" {
  type    = list(string)
  default = ["m6i.large"]
}

variable "node_min" {
  type    = number
  default = 2
}

variable "node_max" {
  type    = number
  default = 6
}

variable "allowed_api_cidrs" {
  description = "CIDRs allowed to reach the public Kubernetes API endpoint. Set this to your office or VPN range."
  type        = list(string)

  validation {
    condition     = !contains(var.allowed_api_cidrs, "0.0.0.0/0")
    error_message = "Do not open the Kubernetes API to 0.0.0.0/0."
  }
}
