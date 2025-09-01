use clap::Parser;
use serde_json::Value;
use std::path::Path;
use tokio;
use zstor_v2::{config::Config, meta::MetaData};

/// CLI tool to decode zstor metadata
#[derive(Parser, Debug)]
#[clap(author, version, about, long_about = None)]
struct Args {
    /// Path to the zstor config file
    #[clap(short, long)]
    config: String,

    /// Path to the file for which to retrieve metadata
    #[clap(short, long, required_unless_present = "all")]
    file: Option<String>,

    /// Retrieve metadata for all stored files
    #[clap(short = 'a', long = "all")]
    all: bool,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();

    // Read and parse the config file
    let config_content = std::fs::read_to_string(&args.config)?;
    let config: Config = toml::from_str(&config_content)?;

    // Create the metastore from the config
    let metastore = zstor_v2::meta::new_metastore(&config).await?;

    if args.all {
        // Try a minimal approach to see if just calling scan_meta_keys fixes the issue
        if let Err(e) = metastore.scan_meta_keys(None, None, None).await {
            eprintln!("Error scanning keys: {}", e);
            std::process::exit(1);
        }

        // Load metadata for all files
        match metastore.object_metas().await {
            Ok(metas) => {
                eprintln!("Found {} metadata entries", metas.len());
                let mut all_metadata = serde_json::Map::new();
                for (key, meta) in metas {
                    eprintln!("Processing key: {}", key);
                    let json_meta = metadata_to_json(&meta);
                    all_metadata.insert(key, json_meta);
                }
                println!(
                    "{}",
                    serde_json::to_string_pretty(&serde_json::Value::Object(all_metadata))?
                );
            }
            Err(e) => {
                eprintln!("Error retrieving metadata for all files: {}", e);
                std::process::exit(1);
            }
        }
    } else {
        // Load metadata for the specified file
        let file_path = Path::new(args.file.as_ref().unwrap());
        let metadata = metastore.load_meta(file_path).await?;

        match metadata {
            Some(meta) => {
                // Convert metadata to JSON for easy machine readability
                let json_meta = metadata_to_json(&meta);
                println!("{}", serde_json::to_string_pretty(&json_meta)?);
            }
            None => {
                eprintln!("No metadata found for file: {}", args.file.unwrap());
                std::process::exit(1);
            }
        }
    }

    Ok(())
}

fn metadata_to_json(meta: &MetaData) -> Value {
    // Convert the metadata to JSON and then manually process it to hex encode all checksums
    let mut json_value = serde_json::to_value(meta).unwrap_or_else(|_| serde_json::Value::Null);

    // Process checksums recursively in the JSON structure
    process_checksums(&mut json_value);

    json_value
}

fn process_checksums(value: &mut Value) {
    match value {
        Value::Object(obj) => {
            // Check if this object has a checksum field
            if let Some(checksum_value) = obj.get("checksum") {
                if let Some(checksum_array) = checksum_value.as_array() {
                    let hex_string: String = checksum_array
                        .iter()
                        .filter_map(|v| v.as_u64())
                        .map(|b| format!("{:02x}", b))
                        .collect();
                    
                    obj.insert("checksum".to_string(), Value::String(hex_string));
                }
            }
            
            // Process all other fields recursively
            for (_, v) in obj.iter_mut() {
                process_checksums(v);
            }
        }
        Value::Array(arr) => {
            // Process all elements in the array recursively
            for v in arr.iter_mut() {
                process_checksums(v);
            }
        }
        _ => {}
    }
}
