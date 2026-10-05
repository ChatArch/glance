require 'yaml'

workflow = YAML.load_file(File.join(__dir__, 'workflows', 'fork-release.yaml'))
verification = workflow.fetch('jobs').fetch('verify-and-build').fetch('steps')
publication = workflow.fetch('jobs').fetch('publish').fetch('steps')
upload = verification.find { |step| step['uses'] == 'actions/upload-artifact@v4' }
release = publication.find { |step| step['uses'] == 'softprops/action-gh-release@v2' }
raise 'missing artifact upload or release step' unless upload && release

assets = %w[dist/glance-chatarch-v*-linux-amd64.tar.gz dist/BUILDINFO.txt dist/SHA256SUMS]
raise 'upload assets differ' unless upload.fetch('with').fetch('path').lines.map(&:strip) == assets
raise 'release assets differ' unless release.fetch('with').fetch('files').lines.map(&:strip) == assets

build = verification.find { |step| step['name'] == 'Build and package Linux amd64' }.fetch('run')
checks = publication.find { |step| step['name'] == 'Verify downloaded checksum' }.fetch('run')
required_build = [
  'version="${TAG}+${commit}"',
  'archive="glance-${TAG}-linux-amd64.tar.gz"',
  'tag=%s\nsource_sha=%s\nbinary_version=%s\narchive=%s\ngoos=linux\ngoarch=amd64\ncgo_enabled=0\n',
  '"$TAG" "$commit" "$version" "$archive" > dist/BUILDINFO.txt',
  'sha256sum "$archive" BUILDINFO.txt > SHA256SUMS'
]
required_checks = [
  'sha256sum -c SHA256SUMS',
  'grep -Fx "tag=${TAG}" BUILDINFO.txt',
  'grep -Fx "source_sha=${commit}" BUILDINFO.txt',
  'grep -Fx "binary_version=${TAG}+${commit}" BUILDINFO.txt',
  'grep -Fx "archive=glance-${TAG}-linux-amd64.tar.gz" BUILDINFO.txt'
]
raise 'missing build metadata or checksum generation' unless required_build.all? { |part| build.include?(part) }
raise 'missing checksum or metadata verification' unless required_checks.all? { |part| checks.include?(part) }

puts 'Fork release assets, BUILDINFO, checksum generation and publication checks match.'
