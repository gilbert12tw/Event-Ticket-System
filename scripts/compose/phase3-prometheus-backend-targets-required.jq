($expected_targets | split(" ")) as $expected
| [.data.result[]?.metric.instance] as $instances
| all($expected[]; . as $target | any($instances[]; . == $target))
