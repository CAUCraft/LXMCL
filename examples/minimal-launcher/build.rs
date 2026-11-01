fn main() {
    let config = slint_build::CompilerConfiguration::new();
    slint_build::compile_with_config("ui/minimal.slint", config).expect("minimal.slint 编译失败");
}
