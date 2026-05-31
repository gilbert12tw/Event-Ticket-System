resource "aws_instance" "app" {
  for_each = {
    for index in range(local.app_node_count) : index => element(local.app_azs, index % length(local.app_azs))
  }

  ami                         = data.aws_ami.amazon_linux.id
  instance_type               = var.app_instance_type
  subnet_id                   = aws_subnet.public[each.value].id
  associate_public_ip_address = true
  vpc_security_group_ids      = [aws_security_group.app.id]
  iam_instance_profile        = aws_iam_instance_profile.ec2.name
  key_name                    = var.ssh_key_name

  user_data = templatefile("${path.module}/templates/app-user-data.sh.tftpl", {
    repository_url            = var.repository_url
    repository_branch         = var.repository_branch
    database_url              = "postgresql://${var.db_username}:${var.db_password}@${aws_db_instance.postgres.address}:5432/${var.db_name}"
    token_signing_secret      = var.token_signing_secret
    provider_token_secret     = var.provider_token_secret
    redis_url                 = "redis://${aws_instance.observability.private_ip}:6379/0"
    queue_url                 = "redis://${aws_instance.observability.private_ip}:6379/1"
    object_storage_endpoint   = "https://s3.${var.aws_region}.amazonaws.com"
    object_storage_region     = var.aws_region
    object_storage_bucket     = aws_s3_bucket.exports.bucket
    object_storage_access_key = aws_iam_access_key.object_store.id
    object_storage_secret_key = aws_iam_access_key.object_store.secret
    mailer_host               = aws_instance.observability.private_ip
    observability_endpoint    = "http://${aws_instance.observability.private_ip}"
    node_index                = each.key
  })

  root_block_device {
    volume_size = 20
    volume_type = "gp3"
    encrypted   = true
  }

  tags = {
    Name = "${var.project_name}-app-${each.key}"
    Role = "app"
  }
}

resource "aws_instance" "observability" {
  ami                         = data.aws_ami.amazon_linux.id
  instance_type               = var.observability_instance_type
  subnet_id                   = values(aws_subnet.public)[0].id
  associate_public_ip_address = true
  vpc_security_group_ids      = [aws_security_group.observability.id]
  iam_instance_profile        = aws_iam_instance_profile.ec2.name
  key_name                    = var.ssh_key_name

  user_data = templatefile("${path.module}/templates/observability-user-data.sh.tftpl", {
    repository_url         = var.repository_url
    repository_branch      = var.repository_branch
    domain_name            = var.domain_name
    grafana_admin_user     = var.grafana_admin_user
    grafana_admin_password = var.grafana_admin_password
  })

  root_block_device {
    volume_size = 30
    volume_type = "gp3"
    encrypted   = true
  }

  tags = {
    Name = "${var.project_name}-observability"
    Role = "observability"
  }
}
