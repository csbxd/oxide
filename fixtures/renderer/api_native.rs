mod api;
use api::{CASES, run};
use std::path::PathBuf;

fn main() {
    let output = PathBuf::from(std::env::args_os().nth(1).expect("API reference directory"));
    std::fs::create_dir_all(&output).unwrap();
    let mut manifest = Vec::new();
    for (case, &(name, source)) in CASES.iter().enumerate() {
        let scratch = output.join(format!("scratch-{case}"));
        let path = scratch.to_str().unwrap();
        let invoke = |output: *mut u8, capacity: usize| unsafe {
            run(case, source.as_ptr(), source.len(), path.as_ptr(), path.len(), output, capacity)
        };
        let size = invoke(std::ptr::null_mut(), 0);
        let mut expected = vec![0; size];
        assert_eq!(invoke(expected.as_mut_ptr(), size), size, "{name}: size changed");
        let mut repeated = vec![0; size];
        assert_eq!(invoke(repeated.as_mut_ptr(), size), size, "{name}: size changed");
        assert_eq!(expected, repeated, "{name}: native output is not deterministic");
        let file = format!("{case}.bin");
        std::fs::write(output.join(&file), expected).unwrap();
        manifest.push(serde_json::json!({"id":case, "name":name, "source":source, "file":file}));
        if scratch.is_dir() {
            std::fs::remove_dir_all(scratch).unwrap();
        } else if scratch.exists() {
            std::fs::remove_file(scratch).unwrap();
        }
        println!("{case}: {name}: {size} bytes");
    }
    std::fs::write(output.join("cases.json"), serde_json::to_vec_pretty(&manifest).unwrap()).unwrap();
    println!("{} public-API cases match two native calls", CASES.len());
}
