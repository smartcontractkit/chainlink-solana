//! This is a tiny arrayvec implementation (https://docs.rs/arrayvec/) that efficiently implements a few common operations
//! We're able to simplify the code significantly due to the elements being Pod/Zeroable.
use bytemuck::Pod;

// #[zero_copy]
#[derive(Debug, Copy, Clone)]
pub struct ArrayVec<T, const CAP: usize>
where
    T: Pod,
{
    xs: [T; CAP],
    len: u32,
}

impl<T: Pod, const CAP: usize> ArrayVec<T, CAP> {
    // const CAPACITY: usize = CAP;

    pub fn new() -> ArrayVec<T, CAP> {
        Self {
            len: 0,
            xs: [T::zeroed(); CAP], // TODO: use zeroed
        }
    }

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
        CAP
    }

    // remaining_capacity

    pub fn push(&mut self, element: T) {
        // TODO: assert len < capacity
        self.xs[self.len as usize] = element;
        self.len += 1;
    }

    pub fn clear(&mut self) {
        // TODO: we can also zero out the array for safety
        self.len = 0;
    }

    pub fn remove(&mut self, index: usize) -> T {
        // TODO: assert len < len
        // TODO: solve if removing last element
        let element = self.xs[index];
        // move index+1..len back by one
        self.xs.copy_within(index + 1..self.len as usize, index);
        // TODO: clear out the last element for safety?
        self.len -= 1;
        element
    }

    #[inline]
    pub fn as_slice(&self) -> &[T] {
        &self.xs[..self.len as usize]
    }

    #[inline]
    pub fn as_mut_slice(&mut self) -> &mut [T] {
        &mut self.xs[..self.len as usize]
    }
}

impl<T, const CAP: usize> std::ops::Deref for ArrayVec<T, CAP>
where
    T: Pod,
{
    type Target = [T];

    #[inline]
    fn deref(&self) -> &Self::Target {
        self.as_slice()
    }
}

impl<T, const CAP: usize> std::ops::DerefMut for ArrayVec<T, CAP>
where
    T: Pod,
{
    #[inline]
    fn deref_mut(&mut self) -> &mut Self::Target {
        self.as_mut_slice()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn remove() {
        let mut vec: ArrayVec<u8, 3> = ArrayVec::new();
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
}
