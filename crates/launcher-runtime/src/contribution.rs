use std::any::{Any, TypeId};
use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use crate::RuntimeError;

/// A typed extension point. Names are globally unique and must retain their type.
pub trait ContributionKey: 'static {
    type Entry: Send + Sync + 'static;
    const NAME: &'static str;
}

/// A snapshot keeps its value alive; removing the scope prevents future discovery.
pub struct Contribution<T> {
    pub id: String,
    pub owner: String,
    pub order: i32,
    pub value: Arc<T>,
}

impl<T> Clone for Contribution<T> {
    fn clone(&self) -> Self {
        Self {
            id: self.id.clone(),
            owner: self.owner.clone(),
            order: self.order,
            value: Arc::clone(&self.value),
        }
    }
}

struct Record {
    id: String,
    owner: String,
    order: i32,
    value: Arc<dyn Any + Send + Sync>,
}
struct Bucket {
    entry_type: TypeId,
    entries: Vec<Record>,
}
#[derive(Default)]
pub(crate) struct ContributionRegistry(Mutex<HashMap<&'static str, Bucket>>);

impl ContributionRegistry {
    pub(crate) fn insert<K: ContributionKey>(
        &self,
        id: String,
        owner: String,
        order: i32,
        entry: K::Entry,
    ) -> Result<(), (K::Entry, RuntimeError)> {
        let mut buckets = self
            .0
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let bucket = buckets.entry(K::NAME).or_insert_with(|| Bucket {
            entry_type: TypeId::of::<K::Entry>(),
            entries: Vec::new(),
        });
        if bucket.entry_type != TypeId::of::<K::Entry>() {
            return Err((
                entry,
                RuntimeError::ContributionTypeMismatch { point: K::NAME },
            ));
        }
        if bucket.entries.iter().any(|item| item.id == id) {
            return Err((
                entry,
                RuntimeError::DuplicateContribution { point: K::NAME, id },
            ));
        }
        bucket.entries.push(Record {
            id,
            owner,
            order,
            value: Arc::new(entry),
        });
        Ok(())
    }

    pub(crate) fn list<K: ContributionKey>(
        &self,
    ) -> Result<Vec<Contribution<K::Entry>>, RuntimeError> {
        let buckets = self
            .0
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let Some(bucket) = buckets.get(K::NAME) else {
            return Ok(Vec::new());
        };
        if bucket.entry_type != TypeId::of::<K::Entry>() {
            return Err(RuntimeError::ContributionTypeMismatch { point: K::NAME });
        }
        let mut result =
            bucket
                .entries
                .iter()
                .map(|record| {
                    Ok(Contribution {
                        id: record.id.clone(),
                        owner: record.owner.clone(),
                        order: record.order,
                        value: record.value.clone().downcast::<K::Entry>().map_err(|_| {
                            RuntimeError::ContributionTypeMismatch { point: K::NAME }
                        })?,
                    })
                })
                .collect::<Result<Vec<_>, RuntimeError>>()?;
        result.sort_by_key(|record| record.order);
        Ok(result)
    }

    pub(crate) fn remove(&self, ids: &[(&'static str, String)]) {
        let mut removed = Vec::new();
        {
            let mut buckets = self
                .0
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner);
            for (point, id) in ids {
                if let Some(bucket) = buckets.get_mut(point)
                    && let Some(index) = bucket.entries.iter().position(|item| item.id == *id)
                {
                    removed.push(bucket.entries.remove(index));
                }
            }
        }
        drop(removed);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{Context, registry::ServiceRegistry};
    struct Entries;
    impl ContributionKey for Entries {
        type Entry = String;
        const NAME: &'static str = "test.entries";
    }
    struct Wrong;
    impl ContributionKey for Wrong {
        type Entry = u32;
        const NAME: &'static str = "test.entries";
    }

    #[test]
    fn ordered_typed_scoped_entries_reject_duplicates_and_closed_contexts() {
        let registry = Arc::new(ServiceRegistry::default());
        let a = Context::with_owner(
            "same-descriptor",
            "first-product-entry".into(),
            registry.clone(),
        );
        let b = Context::with_owner(
            "same-descriptor",
            "second-product-entry".into(),
            registry.clone(),
        );
        a.contribute::<Entries>("a", 10, "A".into()).unwrap();
        b.contribute::<Entries>("b", 0, "B".into()).unwrap();
        a.contribute::<Entries>("c", 10, "C".into()).unwrap();
        assert!(matches!(
            b.contribute::<Entries>("a", 0, "duplicate".into()),
            Err(RuntimeError::DuplicateContribution { .. })
        ));
        assert!(matches!(
            b.contribute::<Wrong>("different", 0, 1),
            Err(RuntimeError::ContributionTypeMismatch { .. })
        ));
        assert!(matches!(
            registry.contributions.list::<Wrong>(),
            Err(RuntimeError::ContributionTypeMismatch { .. })
        ));
        let snapshot = registry.contributions.list::<Entries>().unwrap();
        assert_eq!(
            snapshot
                .iter()
                .map(|item| item.id.as_str())
                .collect::<Vec<_>>(),
            ["b", "a", "c"]
        );
        assert_eq!(snapshot[0].owner, "second-product-entry");
        a.dispose().unwrap();
        assert_eq!(registry.contributions.list::<Entries>().unwrap().len(), 1);
        assert!(matches!(
            a.contribute::<Entries>("late", 0, "late".into()),
            Err(RuntimeError::ContextClosed { .. })
        ));
        assert_eq!(
            snapshot[1].value.as_str(),
            "A",
            "snapshots remain valid after removal"
        );
        b.dispose().unwrap();
        assert!(registry.contributions.list::<Entries>().unwrap().is_empty());
    }
}
