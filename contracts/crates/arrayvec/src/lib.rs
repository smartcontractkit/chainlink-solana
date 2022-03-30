//! This is a tiny arrayvec implementation <https://docs.rs/arrayvec/> that efficiently implements a few common operations
//! We're able to simplify the code significantly due to the elements being Pod/Zeroable.

// use anchor_lang::prelude::*;
// #[zero_copy]
// pub struct $name {
//     pub xs: [$ty; $capacity],
//     pub len: $capacity_ty,
// }
// impl Default for $name {
//     pub fn default() -> Self {
//         Self {
//             len: 0,
//             xs: [<$ty>::default(); $capacity],
//         }
//     }
// }

#[macro_export]
macro_rules! arrayvec {
    ($name:ident, $ty:ty, $capacity_ty:ty) => {
        #[allow(unused)]
        impl $name {
            #[inline(always)]
            pub fn len(&self) -> usize {
                self.len as usize
            }

            #[inline]
            pub fn is_empty(&self) -> bool {
                self.len == 0
            }

            #[inline(always)]
            pub fn capacity(&self) -> usize {
                self.xs.len()
            }

            // remaining_capacity
            #[inline]
            pub fn remaining_capacity(&self) -> usize {
                self.capacity() - self.len()
            }

            pub fn push(&mut self, element: $ty) {
                assert!(self.len() < self.capacity());
                self.xs[self.len as usize] = element;
                self.len += 1;
            }

            pub fn clear(&mut self) {
                self.len = 0;
                // TODO: we can also zero out the array for safety
                // self.xs = [<$ty>::default(); capacity];
            }

            pub fn remove(&mut self, index: usize) -> $ty {
                debug_assert!(index < self.len()); // this will also be asserted by xs[]
                let element = self.xs[index];
                // move index+1..len back by one
                self.xs.copy_within(index + 1..self.len as usize, index);
                // TODO: clear out the last element for safety?
                self.len -= 1;
                element
            }

            pub fn insert(&mut self, index: usize, element: $ty) {
                assert!(self.len() < self.capacity());
                debug_assert!(index <= self.len());

                // move index..len forward by one
                self.xs.copy_within(index..self.len as usize, index + 1);
                self.len += 1;
                self.xs[index] = element;
            }

            #[inline]
            pub fn as_slice(&self) -> &[$ty] {
                &self.xs[..self.len as usize]
            }

            #[inline]
            pub fn as_mut_slice(&mut self) -> &mut [$ty] {
                &mut self.xs[..self.len as usize]
            }

            pub fn extend(&mut self, data: &[$ty]) {
                let len = data.len();
                let offset = self.len();
                self.xs[offset..offset + len].copy_from_slice(&data);
                self.len += len as $capacity_ty;
            }
        }

        impl std::ops::Deref for $name {
            type Target = [$ty];

            #[inline]
            fn deref(&self) -> &Self::Target {
                self.as_slice()
            }
        }

        impl std::ops::DerefMut for $name {
            #[inline]
            fn deref_mut(&mut self) -> &mut Self::Target {
                self.as_mut_slice()
            }
        }
    };
}

#[cfg(test)]
mod tests {
    pub struct ArrayVec {
        pub xs: [u8; 3],
        pub len: u32,
    }
    impl ArrayVec {
        pub fn new() -> Self {
            Self {
                len: 0,
                xs: [u8::default(); 3],
            }
        }
    }
    arrayvec!(ArrayVec, u8, u32);

    #[test]
    fn remove() {
        let mut vec = ArrayVec::new();
        vec.push(1);
        vec.push(2);
        vec.push(3);

        let el = vec.remove(2);
        assert_eq!(el, 3);
        assert_eq!(vec.len(), 2);
        assert_eq!(vec.as_slice(), &[1, 2]);

        let el = vec.remove(0);
        assert_eq!(el, 1);
        assert_eq!(vec.len(), 1);
        assert_eq!(vec.as_slice(), &[2]);
    }

    #[test]
    fn insert() {
        let mut vec = ArrayVec::new();
        vec.insert(0, 3);
        vec.insert(0, 1);
        vec.insert(1, 2);
        assert_eq!(vec.as_slice(), &[1, 2, 3]);
    }

    #[test]
    #[should_panic]
    fn insert_overflow() {
        let mut vec = ArrayVec::new();
        vec.push(1);
        vec.push(2);
        vec.push(3);
        vec.insert(3, 4);
    }
}
