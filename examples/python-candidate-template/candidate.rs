use std::{env, fs};

fn main() {
    let args: Vec<String> = env::args().collect();
    let mut iterations = None;
    let mut input = None;
    let mut index = 1;
    while index < args.len() {
        match args[index].as_str() {
            "--iterations" => {
                index += 1;
                iterations = Some(args[index].parse::<u64>().expect("valid iteration count"));
            }
            "--input" => {
                index += 1;
                input = Some(args[index].clone());
            }
            argument => panic!("unexpected argument: {argument}"),
        }
        index += 1;
    }

    let mut value: i64 = match input {
        Some(path) => fs::read_to_string(path)
            .expect("read input")
            .lines()
            .map(|line| line.parse::<i64>().expect("integer input"))
            .sum(),
        None => 0,
    };
    for iteration in 0..iterations.expect("--iterations is required") {
        value = (value * 31 + iteration as i64 + 7) % 1_000_000_007;
    }
    println!("{value}");
}
