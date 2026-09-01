//! reconfigure-machine — apply declared Incus config to a running container.

use anyhow::Result;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Default, Serialize, Deserialize, PartialEq)]
pub struct Drift {
    pub image: Option<String>,
    pub cpu: Option<u32>,
    pub ram_gb: Option<u32>,
    pub disk_gb: Option<u32>,
}
impl Drift {
    pub fn is_empty(&self) -> bool {
        self.image.is_none()
            && self.cpu.is_none()
            && self.ram_gb.is_none()
            && self.disk_gb.is_none()
    }
}

pub trait ConfigSetter: Send + Sync {
    fn set(&self, host: &str, key: &str, value: &str) -> Result<()>;
}

pub fn apply(
    host: &str,
    drift: &Drift,
    setter: &dyn ConfigSetter,
    dry_run: bool,
) -> Result<Vec<String>> {
    let mut applied = Vec::new();
    if drift.is_empty() {
        return Ok(applied);
    }

    macro_rules! one {
        ($key:expr, $val:expr) => {{
            let v: String = $val;
            if dry_run {
                applied.push(format!("dry: {}={v}", $key));
            } else {
                setter.set(host, $key, &v)?;
                applied.push(format!("{}={v}", $key));
            }
        }};
    }
    if let Some(image) = &drift.image {
        one!("image", image.clone());
    }
    if let Some(cpu) = drift.cpu {
        one!("limits.cpu", cpu.to_string());
    }
    if let Some(ram) = drift.ram_gb {
        one!("limits.memory", format!("{ram}GB"));
    }
    if let Some(disk) = drift.disk_gb {
        one!("limits.disk", format!("{disk}GB"));
    }
    Ok(applied)
}

#[doc(hidden)]
pub mod fakes {
    use super::*;
    use std::sync::Mutex;
    #[derive(Default)]
    pub struct CapturingSetter {
        pub calls: Mutex<Vec<(String, String, String)>>,
    }
    impl ConfigSetter for CapturingSetter {
        fn set(&self, host: &str, key: &str, value: &str) -> Result<()> {
            self.calls
                .lock()
                .unwrap()
                .push((host.into(), key.into(), value.into()));
            Ok(())
        }
    }
}
