variable "vpc_id" {}

variable "subnets" {
    type = list(object({
        name = string
        availability_zone  = string
        cidr_block = string
    }))
}
