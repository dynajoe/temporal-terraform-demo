resource "aws_subnet" "subnet" {
  for_each = { for k, v in var.subnets : v.availability_zone => v }

  vpc_id            = var.vpc_id
  cidr_block        = each.value.cidr_block
  availability_zone = each.value.availability_zone
  tags = {
    Name = each.value.name
  }
}
