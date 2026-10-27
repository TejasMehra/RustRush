// Template Rust candidate for `rustrush candidate`. Same contract as baseline/main.go.
use std::env;
use std::fs;

fn main() {
    let mut args = env::args().skip(1);
    let mut iterations: u64 = 0;
    let mut input = String::new();
    while let Some(argument) = args.next() {
        match argument.as_str() {
            "--iterations" => iterations = args.next().and_then(|v| v.parse().ok()).unwrap_or(0),
            "--input" => input = args.next().unwrap_or_default(),
            _ => {}
        }
    }
    if iterations == 0 || input.is_empty() {
        eprintln!("--iterations and --input are required");
        std::process::exit(2);
    }
    let text = fs::read_to_string(&input).unwrap_or_else(|error| {
        eprintln!("{error}");
        std::process::exit(1);
    });
    let mut numbers: Vec<i64> = Vec::new();
    for line in text.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }
        match line.parse::<i64>() {
            Ok(value) => numbers.push(value),
            Err(error) => {
                eprintln!("{error}");
                std::process::exit(1);
            }
        }
    }
    let mut value: u64 = 0xcbf29ce484222325;
    for _ in 0..iterations {
        for number in &numbers {
            value = (value ^ (*number as u64)).wrapping_mul(0x100000001b3);
        }
    }
    println!("{value}");
}
