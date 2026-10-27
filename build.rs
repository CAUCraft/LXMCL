fn main() {
    println!("cargo:rerun-if-changed=ui/assets/launcher.ico");
    if std::env::var("CARGO_CFG_TARGET_OS").as_deref() == Ok("windows") {
        winresource::WindowsResource::new()
            .set_icon("ui/assets/launcher.ico")
            .set("ProductName", "LXMCL")
            .set("FileDescription", "LXMCL - CAUCRAFT Launcher")
            .compile()
            .expect("Windows icon resource compilation failed");
    }
    let config = slint_build::CompilerConfiguration::new().with_style("fluent".into());
    slint_build::compile_with_config("ui/app.slint", config).expect("Slint UI compilation failed");
}
