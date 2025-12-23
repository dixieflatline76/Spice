package wallpaper

// UpdateImageQuery updates the description of a query.
func (c *Config) UpdateImageQuery(id, description string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	index, err := c.findQueryIndex(id)
	if err != nil {
		return err
	}

	c.Queries[index].Description = description
	c.save()
	return nil
}
