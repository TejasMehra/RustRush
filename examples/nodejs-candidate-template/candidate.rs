// Rust candidate matching baseline.js: same arguments, same recurrence, one output line.
use std::{env, fs, process};

const MODULUS: i64 = 1_000_000_007;

fn run() -> Result<i64, String> {
    let args: Vec<String> = env::args().skip(1).collect();
    let mut iterations: Option<u64> = None;
    let mut input: Option<String> = None;
    let mut index = 0;
    while index < args.len() {
        match args[index].as_str() {
            "--iterations" if index + 1 < args.len() => {
                index += 1;
                iterations = Some(
                    args[index]
                        .parse::<u64>()
                        .map_err(|_| format!("invalid --iterations value: {}", args[index]))?,
                );
            }
            "--input" if index + 1 < args.len() => {
                index += 1;
                input = Some(args[index].clone());
            }
            other => return Err(format!("unexpected argument: {other}")),
        }
        index += 1;
    }
    let iterations = iterations.ok_or("--iterations is required")?;

    let mut value: i64 = 0;
    if let Some(path) = input {
        let text = fs::read_to_string(path).map_err(|error| error.to_string())?;
        for line in text.lines().filter(|line| !line.is_empty()) {
            value += line
                .parse::<i64>()
                .map_err(|_| format!("integer input expected, got: {line}"))?;
        }
    }
    for index in 0..iterations {
        value = (value * 31 + (index % 97) as i64 * 13 + 7) % MODULUS;
    }
    Ok(value)
}

fn main() {
    match run() {
        Ok(value) => println!("{value}"),
        Err(message) => {
            eprintln!("{message}");
            process::exit(2);
        }
    }
}
